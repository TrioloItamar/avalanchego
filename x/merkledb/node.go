// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package merkledb

import (
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/utils/maybe"
)

const HashLength = 32

// Representation of a node stored in the database.
type dbNode struct {
	value    maybe.Maybe[[]byte]
	children map[byte]child
}

type child struct {
	compressedKey Key
	id            ids.ID
	hasValue      bool
}

// node holds additional information on top of the dbNode that makes calculations easier to do
type node struct {
	dbNode
	key         Key
	valueDigest maybe.Maybe[[]byte]
}

// Returns a new node with the given [key] and no value.
// If [parent] isn't nil, the new node is added as a child of [parent].
func newNode(key Key) *node {
	return &node{
		dbNode: dbNode{
			children: make(map[byte]child, 2),
		},
		key: key,
	}
}

// Parse [nodeBytes] to a node and set its key to [key].
func parseNode(tokenConfig TokenConfiguration, key Key, nodeBytes []byte) (*node, error) {
	n := dbNode{}
	if err := codec.decodeDBNode(tokenConfig, nodeBytes, &n); err != nil {
		return nil, err
	}
	result := &node{
		dbNode: n,
		key:    key,
	}

	result.setValueDigest()
	return result, nil
}

// Returns true iff this node has a value.
func (n *node) hasValue() bool {
	return !n.value.IsNothing()
}

// Returns the byte representation of this node.
func (n *node) bytes() []byte {
	return codec.encodeDBNode(&n.dbNode)
}

// Returns and caches the ID of this node.
func (n *node) calculateID(metrics merkleMetrics) ids.ID {
	metrics.HashCalculated()
	hashBytes := codec.encodeHashValues(n)
	return ids.ID(hashing.ComputeHash256(hashBytes))
}

// Set [n]'s value to [val].
func (n *node) setValue(val maybe.Maybe[[]byte]) {
	n.value = val
	n.setValueDigest()
}

func (n *node) setValueDigest() {
	if n.value.IsNothing() || len(n.value.Value()) < HashLength {
		n.valueDigest = n.value
	} else {
		n.valueDigest = maybe.Some(hashing.ComputeHash256(n.value.Value()))
	}
}

// Adds [child] as a child of [n].
// Assumes [child]'s key is valid as a child of [n].
// That is, [n.key] is a prefix of [child.key].
func (n *node) addChild(tc TokenConfiguration, childNode *node) {
	tokenLength := tc.TokenLength(n.key)
	n.setChildEntry(
		childNode.key.Token(tc, tokenLength),
		child{
			compressedKey: childNode.key.Skip(tc, tokenLength+1),
			hasValue:      childNode.hasValue(),
		},
	)
}

// Adds a child to [n] without a reference to the child node.
func (n *node) setChildEntry(index byte, childEntry child) {
	n.children[index] = childEntry
}

// Removes [child] from [n]'s children.
func (n *node) removeChild(tc TokenConfiguration, child *node) {
	delete(n.children, child.key.Token(tc, tc.TokenLength(n.key)))
}

// clone Returns a copy of [n].
// Note: value isn't cloned because it is never edited, only overwritten
// if this ever changes, value will need to be copied as well
// it is safe to clone all fields because they are only written/read while one or both of the db locks are held
func (n *node) clone() *node {
	return &node{
		key: n.key,
		dbNode: dbNode{
			value:    n.value,
			children: maps.Clone(n.children),
		},
		valueDigest: n.valueDigest,
	}
}

// Returns the ProofNode representation of this node.
func (n *node) asProofNode() ProofNode {
	pn := ProofNode{
		Key:         n.key,
		Children:    make(map[byte]ids.ID, len(n.children)),
		ValueOrHash: maybe.Bind(n.valueDigest, slices.Clone[[]byte]),
	}
	for index, entry := range n.children {
		pn.Children[index] = entry.id
	}
	return pn
}
