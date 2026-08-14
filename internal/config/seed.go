package config

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// SeedFromTemplateName is a stable non-zero seed so each saved template
// gets its own kb-{runId} prefix (Save As used to copy smoke's seed → 6a98).
func SeedFromTemplateName(name string) int64 {
	v := hashTemplateName(name)
	// 4-hex run IDs must not collide with stock smoke / object-pressure.
	stock := []int64{StartingTemplate().Naming.Seed.Value, StartingObjectPressure().Naming.Seed.Value}
	for i := 0; i < 64; i++ {
		h := runIDHex(v)
		ok := true
		for _, s := range stock {
			if h == runIDHex(s) {
				ok = false
				break
			}
		}
		if ok {
			return v
		}
		v++
	}
	return v
}

func hashTemplateName(name string) int64 {
	sum := sha256.Sum256([]byte("dasm-burner-template:" + name))
	v := int64(binary.LittleEndian.Uint64(sum[:8]))
	if v < 0 {
		v = -v
	}
	if v == 0 {
		v = 1
	}
	return v
}

func runIDHex(seed int64) string {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(seed))
	sum := sha256.Sum256(buf[:])
	return fmt.Sprintf("%x", sum[:2])
}

// EnsureDistinctTemplateSeed rewrites a clone's seed when it still matches
// the stock smoke / object-pressure seed. Stock templates keep their seeds.
func EnsureDistinctTemplateSeed(c *Config) bool {
	if c == nil {
		return false
	}
	name := c.Metadata.Name
	if name == "" || c.Naming.Seed.Auto {
		return false
	}
	smoke := StartingTemplate()
	op := StartingObjectPressure()
	if name == smoke.Metadata.Name || name == op.Metadata.Name {
		return false
	}
	if c.Naming.Seed.Value == smoke.Naming.Seed.Value || c.Naming.Seed.Value == op.Naming.Seed.Value {
		c.Naming.Seed = Seed{Auto: false, Value: SeedFromTemplateName(name)}
		return true
	}
	return false
}
