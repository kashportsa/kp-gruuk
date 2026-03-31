package tunnel

import (
	"sync"
	"testing"
	"time"
)

func TestMuxRegisterAndResolve(t *testing.T) {
	m := NewMux(5 * time.Second)
	defer m.Close()

	ch, err := m.Register("req-1")
	if err != nil {
		t.Fatal(err)
	}

	resp := &HTTPResponsePayload{StatusCode: 200}
	go func() {
		time.Sleep(10 * time.Millisecond)
		m.Resolve("req-1", resp)
	}()

	got := <-ch
	if got.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", got.StatusCode)
	}
}

func TestMuxDuplicateRegister(t *testing.T) {
	m := NewMux(5 * time.Second)
	defer m.Close()

	_, err := m.Register("req-1")
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.Register("req-1")
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
}

func TestMuxCancel(t *testing.T) {
	m := NewMux(5 * time.Second)
	defer m.Close()

	ch, _ := m.Register("req-1")
	m.Cancel("req-1")

	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed")
	}
}

func TestMuxResolveUnknown(t *testing.T) {
	m := NewMux(5 * time.Second)
	defer m.Close()

	ok := m.Resolve("nonexistent", &HTTPResponsePayload{})
	if ok {
		t.Fatal("expected false for unknown request ID")
	}
}

func TestMuxConcurrent(t *testing.T) {
	m := NewMux(5 * time.Second)
	defer m.Close()

	const n = 50
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := string(rune('A' + idx%26)) + string(rune('0'+idx/26))
			ch, err := m.Register(id)
			if err != nil {
				return
			}
			go func() {
				time.Sleep(time.Millisecond)
				m.Resolve(id, &HTTPResponsePayload{StatusCode: 200 + idx})
			}()
			resp := <-ch
			if resp == nil {
				t.Errorf("nil response for %s", id)
			}
		}(i)
	}
	wg.Wait()
}
