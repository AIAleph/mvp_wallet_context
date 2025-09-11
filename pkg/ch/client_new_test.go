package ch

import "testing"

func TestNewClientVariants(t *testing.T) {
    c1 := New("")
    if c1 == nil || c1.endpoint != "" { t.Fatalf("c1=%+v", c1) }
    c2 := New("http://host:8123/db")
    if c2 == nil || c2.endpoint == "" { t.Fatalf("c2=%+v", c2) }
}

