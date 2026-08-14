package naming

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/dasmlab/dasm-burner/internal/config"
)

const (
	alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	runIDLen = 4
)

const (
	KindNamespace  = "ns"
	KindService    = "svc"
	KindRoute      = "rt"
	KindDeployment = "deploy"
	KindPair       = "pair"
)

// Factory produces deterministic, sortable object names:
//
//	kb-{runID}-{kind}-{seq:05d}-{suffix}
//
// Same seed → same names. RunID is a stable 4-hex fingerprint of the seed.
type Factory struct {
	rng    *rand.Rand
	runID  string
	seed   int64
	prefix map[string]config.NamePrefix
}

func NewFactory(n config.Naming) *Factory {
	seed := n.Seed.Value
	if n.Seed.Auto || seed == 0 {
		seed = time.Now().UnixNano()
	}
	runID := runIDFromSeed(seed)
	return &Factory{
		rng:   rand.New(rand.NewSource(seed)),
		runID: runID,
		seed:  seed,
		prefix: map[string]config.NamePrefix{
			KindNamespace:  withKindDefault(n.Namespace, KindNamespace),
			KindService:    withKindDefault(n.Service, KindService),
			KindRoute:      withKindDefault(n.Route, KindRoute),
			KindDeployment: withKindDefault(n.Deployment, KindDeployment),
			KindPair:       {Prefix: KindPair, RandomLength: 4},
		},
	}
}

func withKindDefault(p config.NamePrefix, kind string) config.NamePrefix {
	if p.Prefix == "" {
		p.Prefix = kind
	}
	if p.RandomLength == 0 {
		p.RandomLength = 4
	}
	return p
}

func runIDFromSeed(seed int64) string {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(seed))
	sum := sha256.Sum256(buf[:])
	return fmt.Sprintf("%x", sum[:2]) // 4 hex chars
}

func PrefixFor(n config.Naming) string {
	return "kb-" + NewFactory(n).RunID()
}

func (f *Factory) RunID() string { return f.runID }
func (f *Factory) Seed() int64   { return f.seed }

// Name returns kb-{runID}-{prefix}-{seq:05d}-{suffix}. seq is 1-based.
func (f *Factory) Name(kind string, seq int) string {
	p := f.prefix[kind]
	if p.Prefix == "" {
		p = config.NamePrefix{Prefix: kind, RandomLength: 4}
	}
	suffix := f.suffix(p.RandomLength)
	return fmt.Sprintf("kb-%s-%s-%05d-%s", f.runID, p.Prefix, seq, suffix)
}

func (f *Factory) suffix(n int) string {
	if n < 1 {
		n = 4
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[f.rng.Intn(len(alphabet))]
	}
	return string(b)
}

func (f *Factory) LabelRun() string {
	return f.runID
}

func SanitizeDNSLabel(s string) string {
	s = strings.ToLower(s)
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}
