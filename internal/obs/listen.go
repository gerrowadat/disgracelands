package obs

import (
	"fmt"
	"net"
)

// listen binds addr, wrapping the error with the address so a failure to bind
// says which listener could not start.
func listen(addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	return ln, nil
}
