package fixture

type Server struct{ Load int }

func Idlest(servers []*Server) *Server {
	best := nil
	for _, s := range servers {
		if best == nil || s.Load < best.Load {
			best = s
		}
	}
	return best
}
