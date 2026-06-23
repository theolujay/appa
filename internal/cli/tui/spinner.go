package tui

import (
	"fmt"
	"os"
	"time"

	"github.com/theolujay/appa/internal/cli/output"
)

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type Spinner struct {
	label  string
	stopCh chan struct{}
	doneCh chan struct{}
	ok     bool
}

func StartSpinner(label string) *Spinner {
	s := &Spinner{
		label:  label,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	go s.run()
	return s
}

func (s *Spinner) run() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	i := 0
	for {
		select {
		case <-s.stopCh:
			fmt.Fprintf(os.Stdout, "\r\033[K")
			if s.ok {
				fmt.Fprintf(os.Stdout, "%s %s\n", output.BoldGreen("✓"), s.label)
			} else {
				fmt.Fprintf(os.Stdout, "%s %s\n", output.BoldRed("✗"), s.label)
			}
			close(s.doneCh)
			return
		case <-ticker.C:
			fmt.Fprintf(os.Stdout, "\r%s %s\033[K", spinFrames[i], s.label)
			i = (i + 1) % len(spinFrames)
		}
	}
}

func (s *Spinner) Stop(ok bool) {
	s.ok = ok
	close(s.stopCh)
	<-s.doneCh
}
