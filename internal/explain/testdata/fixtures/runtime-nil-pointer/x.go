package fixture

// Config holds the settings of a Server.
type Config struct {
	Port int
}

// Server serves on the port of its config.
type Server struct {
	cfg *Config
}

// Port returns the port to listen on.
func (s *Server) Port() int {
	return s.cfg.Port
}
