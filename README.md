# CDN vs Proxy Demo

A comprehensive demonstration comparing CDN (Content Delivery Network) and Proxy-based streaming architectures with real-time visualization and logging.

## Features

### CDN Service
- **File-based video streaming** with looping simulation
- **Traditional HTTP delivery** without FEC
- **Lower packet loss** but higher latency
- **Scalable node architecture**

### Proxy Service (5G + FLUTE)
- **5G broadcast simulation** with FLUTE delivery
- **Raptor FEC implementation** for error correction
- **Multicast delivery** for efficient bandwidth usage
- **Higher packet loss** but lower latency
- **Advanced error recovery** mechanisms

### Real-time Monitoring
- **Live statistics** for both services
- **Real-time logs** via WebSocket
- **Interactive control panel** to add nodes and clients
- **Visual comparison** of performance metrics

## Architecture

```
┌─────────────────┐    ┌─────────────────┐
│   CDN Service   │    │  Proxy Service  │
│                 │    │                 │
│ • File-based    │    │ • 5G Broadcast  │
│ • HTTP Delivery │    │ • FLUTE Delivery│
│ • No FEC        │    │ • Raptor FEC    │
│ • Higher Latency│    │ • Lower Latency │
└─────────────────┘    └─────────────────┘
         │                       │
         └───────────┬───────────┘
                     │
         ┌─────────────────────────┐
         │    HTTP Server          │
         │                         │
         │ • REST API              │
         │ • WebSocket Logs        │
         │ • Static File Serving   │
         └─────────────────────────┘
                     │
         ┌─────────────────────────┐
         │    Frontend UI          │
         │                         │
         │ • Vue.js Application    │
         │ • Real-time Dashboard   │
         │ • Interactive Controls  │
         └─────────────────────────┘
```

## Prerequisites

- Go 1.21 or higher
- Modern web browser with WebSocket support

## Installation

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd mockProxy
   ```

2. **Install Go dependencies**
   ```bash
   go mod tidy
   ```

3. **Run the application**
   ```bash
   go run main.go
   ```

4. **Access the demo**
   Open your browser and navigate to `http://localhost:8080`

## Usage

### Starting the Demo

1. **Launch the application**
   ```bash
   go run main.go
   ```

2. **Open the web interface**
   - Navigate to `http://localhost:8080`
   - The dashboard will automatically connect to the backend

### Adding Nodes and Clients

1. **Add CDN Nodes**
   - Click "Add CDN Node" to create a new CDN server
   - Each node simulates file-based video streaming

2. **Add Proxy Nodes**
   - Click "Add Proxy Node" to create a new Proxy server
   - Each node simulates 5G broadcast with FLUTE delivery

3. **Add Clients**
   - Select a node from the dropdown
   - Click "Add Client" to connect a client to that node
   - Watch real-time streaming logs appear

### Monitoring Performance

- **Real-time Statistics**: View live metrics for both services
- **Performance Comparison**: Compare latency, bandwidth, and packet loss
- **Live Logs**: Monitor streaming activity in real-time
- **Auto-scroll**: Toggle automatic log scrolling

## API Endpoints

### CDN Endpoints
- `POST /api/cdn/nodes` - Add a new CDN node
- `GET /api/cdn/nodes` - Get all CDN nodes
- `POST /api/cdn/nodes/:nodeId/clients` - Add client to CDN node
- `GET /api/cdn/stats` - Get CDN statistics

### Proxy Endpoints
- `POST /api/proxy/nodes` - Add a new Proxy node
- `GET /api/proxy/nodes` - Get all Proxy nodes
- `POST /api/proxy/nodes/:nodeId/clients` - Add client to Proxy node
- `GET /api/proxy/stats` - Get Proxy statistics

### WebSocket
- `GET /ws` - Real-time log streaming

## Technical Details

### CDN Implementation
- **Video Source**: Simulated file-based streaming with looping
- **Delivery Method**: HTTP/HTTPS unicast
- **Error Handling**: No Forward Error Correction (FEC)
- **Latency**: ~50ms (simulated)
- **Packet Loss**: ~0.1% (simulated)

### Proxy Implementation
- **Video Source**: 5G broadcast simulation
- **Delivery Method**: FLUTE multicast delivery
- **Error Handling**: Raptor FEC with 80% recovery rate
- **Latency**: ~20ms (simulated)
- **Packet Loss**: ~2.0% (simulated, with FEC recovery)

### Raptor FEC Configuration
- **Source Symbols**: 100
- **Repair Symbols**: 20
- **Symbol Size**: 1024 bytes
- **Recovery Rate**: 80%

### FLUTE Delivery Configuration
- **Multicast Address**: 239.255.255.250
- **Port Range**: 8080+
- **Session Management**: Automatic heartbeat
- **Transport ID**: Unique per node

## Performance Comparison

| Metric | CDN | Proxy (5G + FLUTE) |
|--------|-----|-------------------|
| Latency | 50ms | 20ms |
| Bandwidth | 100 Mbps | 200 Mbps |
| Packet Loss | 0.1% | 2.0% (with FEC) |
| Scalability | Good | Excellent |
| Error Recovery | None | Raptor FEC |
| Delivery Method | Unicast | Multicast |

## Development

### Project Structure
```
mockProxy/
├── main.go                 # Application entry point
├── go.mod                  # Go module file
├── internal/
│   ├── common/
│   │   └── types.go        # Shared types and structures
│   ├── cdn/
│   │   └── service.go      # CDN service implementation
│   ├── proxy/
│   │   └── service.go      # Proxy service implementation
│   └── server/
│       └── server.go       # HTTP server and API
├── static/
│   ├── index.html          # Main HTML page
│   └── app.js              # Vue.js application
└── README.md               # This file
```

### Adding Features

1. **New Service Types**: Extend the common interfaces
2. **Additional Metrics**: Add new statistics to ServiceStats
3. **Enhanced FEC**: Implement different FEC algorithms
4. **Custom Protocols**: Add new delivery protocols

## Troubleshooting

### Common Issues

1. **Port 8080 already in use**
   ```bash
   # Find and kill the process
   lsof -ti:8080 | xargs kill -9
   ```

2. **WebSocket connection failed**
   - Ensure the server is running
   - Check browser console for errors
   - Verify firewall settings

3. **No logs appearing**
   - Check WebSocket connection status
   - Verify that nodes and clients are added
   - Check browser console for errors

### Debug Mode

To enable debug logging, set the environment variable:
```bash
export DEBUG=true
go run main.go
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- **Raptor FEC**: Based on RFC 5053 and RFC 6330
- **FLUTE**: File Delivery over Unidirectional Transport
- **5G Broadcast**: 3GPP specifications for broadcast services
