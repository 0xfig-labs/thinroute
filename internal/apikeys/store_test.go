package apikeys

import (
	"testing"

	"database/sql"
	_ "modernc.org/sqlite"
)

func TestStoreCreateListAuthenticateRevoke(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create("client")
	if err != nil {
		t.Fatal(err)
	}
	if created.Key == "" || created.ID == "" {
		t.Fatal("created key metadata is incomplete")
	}
	if !store.Authenticate(created.Key) {
		t.Fatal("created key was rejected")
	}
	if store.Authenticate("invalid") {
		t.Fatal("invalid key was accepted")
	}
	keys, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Key != created.Key {
		t.Fatalf("unexpected keys: %+v", keys)
	}
	if err := store.Revoke(created.ID); err != nil {
		t.Fatal(err)
	}
	if store.Authenticate(created.Key) {
		t.Fatal("revoked key was accepted")
	}
}
