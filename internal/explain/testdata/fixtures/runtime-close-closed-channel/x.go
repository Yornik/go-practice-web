package fixture

// Poller polls until Stop is called.
type Poller struct {
	done chan struct{}
}

// NewPoller returns a running Poller.
func NewPoller() *Poller {
	return &Poller{done: make(chan struct{})}
}

// Stop ends the polling.
func (p *Poller) Stop() {
	close(p.done)
}
