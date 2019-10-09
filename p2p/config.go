package p2p

import (
	"crypto/rand"
	"fmt"
	"io"
	mrand "math/rand"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	ma "github.com/multiformats/go-multiaddr"
)

// Config is used to configure a p2p service
type Config struct {
	// Peers used for bootstrapping
	BootstrapNodes []string
	// Listening port
	Port int
	// If 0, random host ID will be generated; If non-0, deterministic ID will be produced
	RandSeed int64
	// Disable bootstrapping altogether. BootstrapNodes has no effect over this.
	NoBootstrap bool
	// Disables MDNS discovery
	NoMdns bool
}

func (c *Config) buildOpts() ([]libp2p.Option, error) {
	ip := "0.0.0.0"

	priv, err := generateKey(c.RandSeed)
	if err != nil {
		return nil, err
	}

	addr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", ip, c.Port))
	if err != nil {
		return nil, err
	}

	connMgr := ConnManager{}

	return []libp2p.Option{
		libp2p.ListenAddrs(addr),
		libp2p.DisableRelay(),
		libp2p.Identity(priv),
		libp2p.NATPortMap(),
		libp2p.Ping(true),
		libp2p.ConnectionManager(connMgr),
	}, nil
}

// generateKey generates a libp2p private key which is used for secure messaging
func generateKey(seed int64) (crypto.PrivKey, error) {
	// If the seed is zero, use real cryptographic randomness. Otherwise, use a
	// deterministic randomness source to make generated keys stay the same
	// across multiple runs
	var r io.Reader
	if seed == 0 {
		r = rand.Reader
	} else {
		r = mrand.New(mrand.NewSource(seed))
	}

	// Generate a key pair for this host. We will use it at least
	// to obtain a valid host ID.
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		return nil, err
	}

	return priv, nil
}
