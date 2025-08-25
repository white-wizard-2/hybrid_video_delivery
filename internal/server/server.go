package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"mockProxy/internal/cdn"
	"mockProxy/internal/common"
	"mockProxy/internal/proxy"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Server struct {
	cdnService    *cdn.CDNService
	proxyService  *proxy.ProxyService
	router        *gin.Engine
	upgrader      websocket.Upgrader
	clients       map[*websocket.Conn]bool
	clientMutexes map[*websocket.Conn]*sync.Mutex
	clientsMutex  sync.RWMutex
	logChan       chan common.LogEntry
	mu            sync.RWMutex
	globalConfig  common.GlobalConfig
}

func NewServer(cdnService *cdn.CDNService, proxyService *proxy.ProxyService) *Server {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for demo
		},
	}

	server := &Server{
		cdnService:    cdnService,
		proxyService:  proxyService,
		upgrader:      upgrader,
		clients:       make(map[*websocket.Conn]bool),
		clientMutexes: make(map[*websocket.Conn]*sync.Mutex),
		logChan:       make(chan common.LogEntry, 1000),
	}

	server.setupRoutes()
	return server
}

func (s *Server) setupRoutes() {
	router := gin.Default()

	// CORS configuration
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization"}
	router.Use(cors.New(config))

	// API routes
	api := router.Group("/api")
	{
		// CDN routes
		cdn := api.Group("/cdn")
		{
			cdn.POST("/nodes", s.addCDNNode)
			cdn.GET("/nodes", s.getCDNNodes)
			cdn.POST("/nodes/:nodeId/clients", s.addCDNClient)
			cdn.GET("/stats", s.getCDNStats)
			cdn.GET("/nodes/:nodeId/manifest/:format", s.getCDNManifest)
		}

		// Proxy routes
		proxy := api.Group("/proxy")
		{
			proxy.POST("/nodes", s.addProxyNode)
			proxy.GET("/nodes", s.getProxyNodes)
			proxy.POST("/nodes/:nodeId/clients", s.addProxyClient)
			proxy.GET("/stats", s.getProxyStats)
			proxy.GET("/nodes/:nodeId/manifest/:format", s.getProxyManifest)
		}

		// Comparison routes
		api.GET("/comparison", s.getComparison)

		// Configuration routes
		api.GET("/config", s.getConfig)
		api.PUT("/config", s.updateConfig)
	}

	// WebSocket for real-time logs
	router.GET("/ws", s.handleWebSocket)

	// Serve static files (frontend)
	router.Static("/static", "./static")
	router.StaticFile("/", "./static/index.html")
	router.StaticFile("/app.js", "./static/app.js")
	router.StaticFile("/sample.mp4", "./sample.mp4")
	router.GET("/test", func(c *gin.Context) {
		c.File("./static/test.html")
	})
	router.GET("/vue-test", func(c *gin.Context) {
		c.File("./static/vue-test.html")
	})

	s.router = router
}

// getClientMutex returns a mutex for a specific WebSocket client to prevent concurrent writes
func (s *Server) getClientMutex(client *websocket.Conn) *sync.Mutex {
	s.clientsMutex.Lock()
	defer s.clientsMutex.Unlock()

	if mutex, exists := s.clientMutexes[client]; exists {
		return mutex
	}

	// Create new mutex for this client
	mutex := &sync.Mutex{}
	s.clientMutexes[client] = mutex
	return mutex
}

func (s *Server) Start() {
	log.Println("Server starting on :8080")
	if err := s.router.Run(":8080"); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

func (s *Server) Stop() {
	log.Println("Server stopping...")
}

// WebSocket handler for real-time logs
func (s *Server) handleWebSocket(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	s.clientsMutex.Lock()
	s.clients[conn] = true
	s.clientMutexes[conn] = &sync.Mutex{}
	s.clientsMutex.Unlock()

	// Start broadcasting logs if not already started
	if len(s.clients) == 1 {
		go s.broadcastLogs()
	}

	// Handle client disconnect
	defer func() {
		s.clientsMutex.Lock()
		delete(s.clients, conn)
		delete(s.clientMutexes, conn)
		s.clientsMutex.Unlock()
		conn.Close()
	}()

	// Keep connection alive
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (s *Server) broadcastLogs() {
	// Broadcast CDN logs
	go func() {
		for logEntry := range s.cdnService.GetLogs() {
			s.logChan <- logEntry
		}
	}()

	// Broadcast Proxy logs
	go func() {
		for logEntry := range s.proxyService.GetLogs() {
			s.logChan <- logEntry
		}
	}()

	// Single broadcaster for all logs
	go func() {
		for logEntry := range s.logChan {
			s.broadcastLog(logEntry)
		}
	}()
}

func (s *Server) broadcastLog(logEntry common.LogEntry) {
	data, err := json.Marshal(logEntry)
	if err != nil {
		log.Printf("Failed to marshal log entry: %v", err)
		return
	}

	s.clientsMutex.RLock()
	clients := make([]*websocket.Conn, 0, len(s.clients))
	for client := range s.clients {
		clients = append(clients, client)
	}
	s.clientsMutex.RUnlock()

	for _, client := range clients {
		// Use a mutex to prevent concurrent writes to the same WebSocket connection
		clientMutex := s.getClientMutex(client)
		clientMutex.Lock()
		err := client.WriteMessage(websocket.TextMessage, data)
		clientMutex.Unlock()

		if err != nil {
			log.Printf("Failed to send log to client: %v", err)
			s.clientsMutex.Lock()
			delete(s.clients, client)
			delete(s.clientMutexes, client)
			s.clientsMutex.Unlock()
			client.Close()
		}
	}
}

// API Handlers
func (s *Server) addCDNNode(c *gin.Context) {
	nodeID := s.cdnService.AddNode()
	c.JSON(http.StatusOK, gin.H{"nodeId": nodeID})
}

func (s *Server) getCDNNodes(c *gin.Context) {
	nodes := s.cdnService.GetNodes()
	c.JSON(http.StatusOK, nodes)
}

func (s *Server) addCDNClient(c *gin.Context) {
	nodeID := c.Param("nodeId")
	clientID := s.cdnService.AddClient(nodeID)
	if clientID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"clientId": clientID})
}

func (s *Server) getCDNStats(c *gin.Context) {
	stats := s.cdnService.GetStats()
	c.JSON(http.StatusOK, stats)
}

func (s *Server) addProxyNode(c *gin.Context) {
	nodeID := s.proxyService.AddNode()
	c.JSON(http.StatusOK, gin.H{"nodeId": nodeID})
}

func (s *Server) getProxyNodes(c *gin.Context) {
	nodes := s.proxyService.GetNodes()
	c.JSON(http.StatusOK, nodes)
}

func (s *Server) addProxyClient(c *gin.Context) {
	nodeID := c.Param("nodeId")
	clientID := s.proxyService.AddClient(nodeID)
	if clientID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"clientId": clientID})
}

func (s *Server) getProxyStats(c *gin.Context) {
	stats := s.proxyService.GetStats()
	c.JSON(http.StatusOK, stats)
}

func (s *Server) getComparison(c *gin.Context) {
	cdnStats := s.cdnService.GetStats()
	proxyStats := s.proxyService.GetStats()

	comparison := gin.H{
		"cdn":       cdnStats,
		"proxy":     proxyStats,
		"timestamp": time.Now(),
	}

	c.JSON(http.StatusOK, comparison)
}

func (s *Server) getConfig(c *gin.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c.JSON(http.StatusOK, s.globalConfig)
}

func (s *Server) updateConfig(c *gin.Context) {
	var config common.GlobalConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s.mu.Lock()
	s.globalConfig = config
	s.mu.Unlock()

	// Update services with new configuration
	s.cdnService.UpdateConfig(config)
	s.proxyService.UpdateConfig(config)

	c.JSON(http.StatusOK, gin.H{"message": "Configuration updated successfully"})
}

// CMAF Manifest Handlers
func (s *Server) getCDNManifest(c *gin.Context) {
	nodeID := c.Param("nodeId")
	format := c.Param("format")

	nodes := s.cdnService.GetNodes()
	node, exists := nodes[nodeID]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
		return
	}

	if node.Stream == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No CMAF stream available"})
		return
	}

	manifest, err := node.Stream.GetManifest(format)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "application/xml")
	if format == "hls" {
		c.Header("Content-Type", "application/vnd.apple.mpegurl")
	}
	c.String(http.StatusOK, manifest)
}

func (s *Server) getProxyManifest(c *gin.Context) {
	nodeID := c.Param("nodeId")
	format := c.Param("format")

	nodes := s.proxyService.GetNodes()
	node, exists := nodes[nodeID]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
		return
	}

	if node.Stream == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No CMAF stream available"})
		return
	}

	manifest, err := node.Stream.GetManifest(format)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "application/xml")
	if format == "hls" {
		c.Header("Content-Type", "application/vnd.apple.mpegurl")
	}
	c.String(http.StatusOK, manifest)
}
