package seat

import "testing"

func TestDefaultLayoutCounts(t *testing.T) {
	layout := DefaultLayout()
	if got, want := len(layout), 385; got != want {
		t.Fatalf("len(DefaultLayout()) = %d, want %d", got, want)
	}

	blocked := 0
	seen := map[string]bool{}
	for _, def := range layout {
		key := def.Zone + "/" + itoa(def.RowNo) + "/" + itoa(def.ColNo)
		if seen[key] {
			t.Fatalf("duplicate seat %s", key)
		}
		seen[key] = true
		if def.IsBlocked {
			blocked++
		}
	}
	if got, want := blocked, 24; got != want {
		t.Fatalf("blocked seats = %d, want %d", got, want)
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[i:])
}
