package fixture

// Poller polls until Stop is called.
type Poller struct {
	done chan struct{}
}

// Stop ends the polling.
func (p *Poller) Stop() {
	close(p.done)
}
