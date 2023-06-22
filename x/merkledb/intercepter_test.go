// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package merkledb

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"

	"github.com/ava-labs/avalanchego/ids"
)

func Test_Intercepter_empty_db(t *testing.T) {
	require := require.New(t)

	db, err := getBasicDB()
	require.NoError(err)

	testIntercepter(require, db, []KeyChange{
		{
			Key:   []byte{0},
			Value: Some([]byte{0, 1, 2}),
		},
		{
			Key:   []byte{1},
			Value: Some([]byte{1, 2}),
		},
	})
}

func Test_Intercepter_non_empty_initial_db(t *testing.T) {
	require := require.New(t)

	db, err := getBasicDB()
	require.NoError(err)

	require.NoError(db.Put([]byte{0}, []byte{2}))

	testIntercepter(require, db, []KeyChange{
		{
			Key:   []byte{0},
			Value: Some([]byte{0, 1, 2}),
		},
		{
			Key:   []byte{1},
			Value: Some([]byte{1, 2}),
		},
	})
}

func Test_Intercepter_non_empty_initial_db_with_delete(t *testing.T) {
	require := require.New(t)

	db, err := getBasicDB()
	require.NoError(err)

	require.NoError(db.Put([]byte{0}, []byte{2}))

	testIntercepter(require, db, []KeyChange{
		{
			Key:   []byte{0},
			Value: Nothing[[]byte](),
		},
		{
			Key:   []byte{1},
			Value: Some([]byte{1, 2}),
		},
	})
}

type proofNode struct {
	key  []byte
	node []byte
}

func testIntercepter(require *require.Assertions, db *merkleDB, changes []KeyChange) {
	startRootID, startRoot, valueProofs, pathProofs, endRootID := build(require, db, changes)
	verify(require, startRootID, startRoot, valueProofs, pathProofs, changes, endRootID)
}

func build(
	require *require.Assertions,
	db *merkleDB,
	changes []KeyChange,
) (
	ids.ID,
	[]byte,
	[]*Proof,
	[]*PathProof,
	ids.ID,
) {
	ctx := context.Background()
	startRootID, err := db.GetMerkleRoot(ctx)
	require.NoError(err)

	startRootBytes, err := db.root.marshal()
	require.NoError(err)

	viewIntf, err := db.NewView()
	require.NoError(err)
	view := viewIntf.(*trieView)

	var lock sync.Mutex
	view.proverIntercepter = &trieViewProverIntercepter{
		lock:   &lock,
		values: make(map[path]*Proof),
		nodes:  make(map[path]*PathProof),
	}
	for _, change := range changes {
		if change.Value.IsNothing() {
			require.NoError(view.Remove(ctx, change.Key))
		} else {
			require.NoError(view.Insert(ctx, change.Key, change.Value.Value()))
		}
	}

	expectedNewRoot, err := view.GetMerkleRoot(ctx)
	require.NoError(err)

	valueProofs := maps.Values(view.proverIntercepter.values)
	pathProofs := maps.Values(view.proverIntercepter.nodes)

	return startRootID, startRootBytes, valueProofs, pathProofs, expectedNewRoot
}

func verify(
	require *require.Assertions,
	startRootID ids.ID,
	startRootBytes []byte,
	valueProofs []*Proof,
	pathProofs []*PathProof,
	changes []KeyChange,
	expectedRootID ids.ID,
) {
	startRoot, err := parseNode(RootPath, startRootBytes)
	require.NoError(err)

	ctx := context.Background()
	for _, proof := range valueProofs {
		require.NoError(proof.Verify(ctx, startRootID))
	}
	for _, proof := range pathProofs {
		require.NoError(proof.Verify(ctx, startRootID))
	}

	values := make(map[path]Maybe[[]byte])
	for _, proof := range valueProofs {
		values[newPath(proof.Key)] = proof.Value
	}

	nodes := make(map[path]Maybe[*node])
	for _, proof := range pathProofs {
		key := proof.KeyPath.deserialize()
		lastNode := proof.Path[len(proof.Path)-1]

		if !lastNode.KeyPath.Equal(proof.KeyPath) {
			nodes[key] = Nothing[*node]()
			continue
		}

		n := &node{
			dbNode: dbNode{
				value:    proof.Value,
				children: lastNode.Children,
			},
			key:         key,
			valueDigest: lastNode.ValueOrHash,
		}

		nodes[key] = Some(n)
	}

	view := &trieView{
		root: startRoot,
		db: &merkleDB{
			metrics: &mockMetrics{},
			tracer:  newNoopTracer(),
		},
		parentTrie:            nil,
		changes:               newChangeSummary(1),
		estimatedSize:         1,
		unappliedValueChanges: make(map[path]Maybe[[]byte], 1),

		verifierIntercepter: &trieViewVerifierIntercepter{
			values: values,
			nodes:  nodes,
		},
	}

	for _, change := range changes {
		if change.Value.IsNothing() {
			require.NoError(view.Remove(ctx, change.Key))
		} else {
			require.NoError(view.Insert(ctx, change.Key, change.Value.Value()))
		}
	}

	newRoot, err := view.GetMerkleRoot(ctx)
	require.NoError(err)
	require.Equal(expectedRootID, newRoot)
}