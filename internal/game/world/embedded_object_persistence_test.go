package world

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/persistence"

	"github.com/stretchr/testify/require"
)

func TestPersistWorldObjectRejectsNilDatabaseBeforeSerialization(t *testing.T) {
	w := ecs.NewWorldForTesting()
	h := w.Spawn(1, nil)
	factory := NewObjectFactory(nil)
	require.EqualError(t, factory.PersistWorldObjectNow(nil, w, h), "invalid persist target")
	var postgres *persistence.Postgres
	require.EqualError(t, factory.PersistWorldObjectNow(postgres, w, h), "invalid persist target")
}
