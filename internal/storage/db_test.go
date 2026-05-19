package storage

import "testing"

func TestMigrate_Idempotent(t *testing.T) {
	t.Skip("PostgreSQL required: run `akama db start` and set POSTGRES_TEST_URL")
}

func TestJobCRUD(t *testing.T) {
	t.Skip("PostgreSQL required: run `akama db start` and set POSTGRES_TEST_URL")
}

func TestConnectionCRUD(t *testing.T) {
	t.Skip("PostgreSQL required: run `akama db start` and set POSTGRES_TEST_URL")
}

func TestConversationState(t *testing.T) {
	t.Skip("PostgreSQL required: run `akama db start` and set POSTGRES_TEST_URL")
}
