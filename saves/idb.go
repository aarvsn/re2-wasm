//go:build js && wasm

// saves/idb.go contains the browser-backed SaveStore. It uses the
// IndexedDB API via syscall/js. The implementation is deliberately minimal:
// one object store ("saves") keyed by slot number, with values being the raw
// save byte arrays.
//
// All IndexedDB calls are wrapped in Promise-to-channel bridges so the Go
// side can await them synchronously inside engine.SaveStore methods.
package saves

import (
	"context"
	"errors"
	"fmt"
	"syscall/js"
)

// IDBStore is the IndexedDB-backed SaveStore. The zero value is not usable;
// call Open.
type IDBStore struct {
	db js.Value
}

// OpenIDB opens (or creates) the "re2-wasm" database with a single "saves"
// object store. It blocks until the open completes or fails.
func OpenIDB() (*IDBStore, error) {
	if !js.Global().Get("indexedDB").Truthy() {
		return nil, errors.New("saves: IndexedDB is not available in this browser")
	}
	req := js.Global().Get("indexedDB").Call("open", "re2-wasm", 1)
	req.Set("onupgradeneeded", js.FuncOf(func(this js.Value, args []js.Value) any {
		db := this.Get("result")
		if !db.Call("objectStoreNames").Call("contains", "saves").Bool() {
			db.Call("createObjectStore", "saves", map[string]any{"keyPath": "slot"})
		}
		return nil
	}))
	db, err := await(req)
	if err != nil {
		return nil, fmt.Errorf("saves: open IDB: %w", err)
	}
	return &IDBStore{db: db}, nil
}

// Load implements engine.SaveStore.
func (s *IDBStore) Load(_ context.Context, slot int) ([]byte, error) {
	if err := checkSlot(slot); err != nil {
		return nil, err
	}
	tx := s.db.Call("transaction", "saves", "readonly")
	store := tx.Call("objectStore", "saves")
	req := store.Call("get", slot)
	v, err := await(req)
	if err != nil {
		return nil, err
	}
	if !v.Truthy() {
		return nil, ErrSlotEmpty{Slot: slot}
	}
	// v is {slot, data}; extract data as a Uint8Array copy.
	dataJS := v.Get("data")
	if !dataJS.Truthy() {
		return nil, ErrSlotEmpty{Slot: slot}
	}
	return uint8ArrayToBytes(dataJS), nil
}

// Save implements engine.SaveStore.
func (s *IDBStore) Save(_ context.Context, slot int, data []byte) error {
	if err := checkSlot(slot); err != nil {
		return err
	}
	if data == nil {
		return errors.New("saves: data is nil")
	}
	tx := s.db.Call("transaction", "saves", "readwrite")
	store := tx.Call("objectStore", "saves")
	rec := js.Global().Get("Object").New()
	rec.Set("slot", slot)
	rec.Set("data", bytesToUint8Array(data))
	req := store.Call("put", rec)
	_, err := await(req)
	return err
}

// List implements engine.SaveStore.
func (s *IDBStore) List(_ context.Context) ([]int, error) {
	tx := s.db.Call("transaction", "saves", "readonly")
	store := tx.Call("objectStore", "saves")
	req := store.Call("getAllKeys")
	v, err := await(req)
	if err != nil {
		return nil, err
	}
	out := make([]int, 0, v.Length())
	for i := 0; i < v.Length(); i++ {
		out = append(out, v.Index(i).Int())
	}
	return out, nil
}

// Export implements engine.SaveStore.
func (s *IDBStore) Export(slot int) ([]byte, error) {
	return s.Load(context.Background(), slot)
}

// Import implements engine.SaveStore.
func (s *IDBStore) Import(slot int, data []byte) error {
	return s.Save(context.Background(), slot, data)
}

// await blocks until the given IDBRequest settles and returns either its
// result (on success) or an error wrapping the request's error.
func await(req js.Value) (js.Value, error) {
	done := make(chan struct{})
	var result js.Value
	var err error
	req.Set("onsuccess", js.FuncOf(func(this js.Value, args []js.Value) any {
		result = this.Get("result")
		close(done)
		return nil
	}))
	req.Set("onerror", js.FuncOf(func(this js.Value, args []js.Value) any {
		e := this.Get("error")
		if e.Truthy() {
			err = errors.New(e.String())
		} else {
			err = errors.New("saves: unknown IDB error")
		}
		close(done)
		return nil
	}))
	<-done
	return result, err
}

// bytesToUint8Array copies a Go []byte into a fresh JS Uint8Array.
func bytesToUint8Array(b []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)
	return arr
}

// uint8ArrayToBytes copies a JS Uint8Array (or any ArrayBuffer view) into a
// fresh Go []byte.
func uint8ArrayToBytes(v js.Value) []byte {
	n := v.Get("byteLength").Int()
	out := make([]byte, n)
	js.CopyBytesToGo(out, v)
	return out
}
