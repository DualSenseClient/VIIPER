package usb

import (
	"context"
	"time"
)

const endpointDiagnosticsInterval = 5 * time.Second

// USBIPDiagnosticsSnapshot is an externally reachable aggregate view of all
// currently attached USB/IP endpoint schedulers. Calling it is diagnostic-only
// work: endpoint and response hot paths only update fixed atomic histograms.
type USBIPDiagnosticsSnapshot struct {
	CapturedAt  time.Time
	Connections []USBIPConnectionDiagnostics
}

// EndpointDiagnosticsSnapshot returns aggregate queue, service-lateness, and
// response-serialization distributions for active USB/IP connections.
func (s *Server) EndpointDiagnosticsSnapshot() USBIPDiagnosticsSnapshot {
	s.diagnosticsMu.Lock()
	connections := make([]*endpointSchedulers, 0, len(s.diagnosticConnections))
	for connection := range s.diagnosticConnections {
		connections = append(connections, connection)
	}
	s.diagnosticsMu.Unlock()

	snapshot := USBIPDiagnosticsSnapshot{
		CapturedAt:  time.Now(),
		Connections: make([]USBIPConnectionDiagnostics, 0, len(connections)),
	}
	for _, connection := range connections {
		snapshot.Connections = append(snapshot.Connections, connection.snapshot())
	}
	return snapshot
}

func (s *Server) registerEndpointSchedulers(schedulers *endpointSchedulers) {
	s.diagnosticsMu.Lock()
	s.diagnosticConnections[schedulers] = struct{}{}
	s.diagnosticsMu.Unlock()
}

func (s *Server) unregisterEndpointSchedulers(schedulers *endpointSchedulers) {
	s.diagnosticsMu.Lock()
	delete(s.diagnosticConnections, schedulers)
	s.diagnosticsMu.Unlock()
}

func (s *Server) emitEndpointDiagnostics(
	ctx context.Context,
	schedulers *endpointSchedulers,
) {
	if !s.config.EndpointDiagnostics || s.logger == nil {
		return
	}
	ticker := time.NewTicker(endpointDiagnosticsInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.logger.Info("USB/IP endpoint scheduling diagnostics",
				"snapshot", schedulers.snapshot())
		}
	}
}
