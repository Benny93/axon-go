// Package graph provides the in-memory knowledge graph for Axon.
//
// It provides a lightweight, map-backed graph that stores GraphNode and
// GraphRelationship instances with O(1) lookups by ID. Secondary indexes
// on label, relationship type, and adjacency lists ensure that queries
// scale linearly with the result set rather than the total graph size.
package graph

import (
	"sync"
)

// KnowledgeGraph is an in-memory directed graph of code-level entities
// and their relationships.
//
// Nodes are keyed by their ID string; relationships are keyed likewise.
// Removing a node cascades to any relationship where the node appears as
// source or target.
//
// All query methods are backed by secondary indexes so that lookups by
// label, relationship type, or adjacency are O(result) rather than O(graph).
type KnowledgeGraph struct {
	mu            sync.RWMutex
	nodes         map[string]*GraphNode
	relationships map[string]*GraphRelationship

	// Secondary indexes — kept in sync by add/remove helpers.
	byLabel     map[NodeLabel]map[string]*GraphNode
	byRelType   map[RelType]map[string]*GraphRelationship
	outgoing    map[string]map[string]*GraphRelationship
	incoming    map[string]map[string]*GraphRelationship
}

// NewKnowledgeGraph creates a new empty knowledge graph.
func NewKnowledgeGraph() *KnowledgeGraph {
	return &KnowledgeGraph{
		nodes:         make(map[string]*GraphNode),
		relationships: make(map[string]*GraphRelationship),
		byLabel:       make(map[NodeLabel]map[string]*GraphNode),
		byRelType:     make(map[RelType]map[string]*GraphRelationship),
		outgoing:      make(map[string]map[string]*GraphRelationship),
		incoming:      make(map[string]map[string]*GraphRelationship),
	}
}

// NodeCount returns the number of nodes without list materialization.
func (g *KnowledgeGraph) NodeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

// RelationshipCount returns the number of relationships without list materialization.
func (g *KnowledgeGraph) RelationshipCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.relationships)
}

// CountNodesByLabel returns the count of nodes with the given label.
func (g *KnowledgeGraph) CountNodesByLabel(label NodeLabel) int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if nodes, ok := g.byLabel[label]; ok {
		return len(nodes)
	}
	return 0
}

// IterNodes returns a channel that yields all nodes.
func (g *KnowledgeGraph) IterNodes() <-chan *GraphNode {
	g.mu.RLock()
	ch := make(chan *GraphNode, len(g.nodes))
	for _, node := range g.nodes {
		ch <- node
	}
	close(ch)
	g.mu.RUnlock()
	return ch
}

// IterRelationships returns a channel that yields all relationships.
func (g *KnowledgeGraph) IterRelationships() <-chan *GraphRelationship {
	g.mu.RLock()
	ch := make(chan *GraphRelationship, len(g.relationships))
	for _, rel := range g.relationships {
		ch <- rel
	}
	close(ch)
	g.mu.RUnlock()
	return ch
}

// AddNode adds a node to the graph, replacing any existing node with the same ID.
// If the node's label differs from an existing node, the old label index is updated.
func (g *KnowledgeGraph) AddNode(node *GraphNode) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Remove old node from label index if label changed
	if old, ok := g.nodes[node.ID]; ok && old.Label != node.Label {
		delete(g.byLabel[old.Label], node.ID)
	}

	g.nodes[node.ID] = node

	if g.byLabel[node.Label] == nil {
		g.byLabel[node.Label] = make(map[string]*GraphNode)
	}
	g.byLabel[node.Label][node.ID] = node
}

// GetNode returns the node with the given ID, or nil if it does not exist.
func (g *KnowledgeGraph) GetNode(nodeID string) *GraphNode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.nodes[nodeID]
}

// RemoveNode removes a node and cascade-deletes all relationships that reference it.
// Returns true if the node existed and was removed, false otherwise.
func (g *KnowledgeGraph) RemoveNode(nodeID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	node, ok := g.nodes[nodeID]
	if !ok {
		return false
	}

	delete(g.nodes, nodeID)
	delete(g.byLabel[node.Label], nodeID)

	g.cascadeRelationshipsForNode(nodeID)
	return true
}

// RemoveNodesByFile removes every node whose FilePath matches and cascade-deletes relationships.
// Returns the number of nodes removed.
func (g *KnowledgeGraph) RemoveNodesByFile(filePath string) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Collect IDs to remove
	idsToRemove := make([]string, 0)
	for id, node := range g.nodes {
		if node.FilePath == filePath {
			idsToRemove = append(idsToRemove, id)
		}
	}

	if len(idsToRemove) == 0 {
		return 0
	}

	// Remove nodes
	for _, id := range idsToRemove {
		node := g.nodes[id]
		delete(g.nodes, id)
		delete(g.byLabel[node.Label], id)
	}

	// Cascade delete relationships
	for _, id := range idsToRemove {
		g.cascadeRelationshipsForNode(id)
	}

	return len(idsToRemove)
}

// AddRelationship adds a relationship to the graph, replacing any existing relationship with the same ID.
func (g *KnowledgeGraph) AddRelationship(rel *GraphRelationship) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Remove old relationship from indexes
	if old, ok := g.relationships[rel.ID]; ok {
		delete(g.byRelType[old.Type], rel.ID)
		delete(g.outgoing[old.Source], rel.ID)
		delete(g.incoming[old.Target], rel.ID)
	}

	g.relationships[rel.ID] = rel

	if g.byRelType[rel.Type] == nil {
		g.byRelType[rel.Type] = make(map[string]*GraphRelationship)
	}
	g.byRelType[rel.Type][rel.ID] = rel

	if g.outgoing[rel.Source] == nil {
		g.outgoing[rel.Source] = make(map[string]*GraphRelationship)
	}
	g.outgoing[rel.Source][rel.ID] = rel

	if g.incoming[rel.Target] == nil {
		g.incoming[rel.Target] = make(map[string]*GraphRelationship)
	}
	g.incoming[rel.Target][rel.ID] = rel
}

// GetNodesByLabel returns all nodes with the given label.
func (g *KnowledgeGraph) GetNodesByLabel(label NodeLabel) []*GraphNode {
	g.mu.RLock()
	defer g.mu.RUnlock()

	nodes, ok := g.byLabel[label]
	if !ok {
		return nil
	}

	result := make([]*GraphNode, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, node)
	}
	return result
}

// GetRelationshipsByType returns all relationships with the given type.
func (g *KnowledgeGraph) GetRelationshipsByType(relType RelType) []*GraphRelationship {
	g.mu.RLock()
	defer g.mu.RUnlock()

	rels, ok := g.byRelType[relType]
	if !ok {
		return nil
	}

	result := make([]*GraphRelationship, 0, len(rels))
	for _, rel := range rels {
		result = append(result, rel)
	}
	return result
}

// GetOutgoing returns relationships originating from the given node ID.
// If relType is provided, only relationships of that type are returned.
func (g *KnowledgeGraph) GetOutgoing(nodeID string, relType ...RelType) []*GraphRelationship {
	g.mu.RLock()
	defer g.mu.RUnlock()

	rels, ok := g.outgoing[nodeID]
	if !ok {
		return nil
	}

	if len(relType) > 0 && relType[0] != "" {
		result := make([]*GraphRelationship, 0)
		for _, rel := range rels {
			if rel.Type == relType[0] {
				result = append(result, rel)
			}
		}
		return result
	}

	result := make([]*GraphRelationship, 0, len(rels))
	for _, rel := range rels {
		result = append(result, rel)
	}
	return result
}

// GetIncoming returns relationships targeting the given node ID.
// If relType is provided, only relationships of that type are returned.
func (g *KnowledgeGraph) GetIncoming(nodeID string, relType ...RelType) []*GraphRelationship {
	g.mu.RLock()
	defer g.mu.RUnlock()

	rels, ok := g.incoming[nodeID]
	if !ok {
		return nil
	}

	if len(relType) > 0 && relType[0] != "" {
		result := make([]*GraphRelationship, 0)
		for _, rel := range rels {
			if rel.Type == relType[0] {
				result = append(result, rel)
			}
		}
		return result
	}

	result := make([]*GraphRelationship, 0, len(rels))
	for _, rel := range rels {
		result = append(result, rel)
	}
	return result
}

// HasIncoming returns true if the node has any incoming relationship of the given type.
func (g *KnowledgeGraph) HasIncoming(nodeID string, relType RelType) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	rels, ok := g.incoming[nodeID]
	if !ok {
		return false
	}

	for _, rel := range rels {
		if rel.Type == relType {
			return true
		}
	}
	return false
}

// GetCallees returns nodes called by the given node.
func (g *KnowledgeGraph) GetCallees(nodeID string) []*GraphNode {
	g.mu.RLock()
	defer g.mu.RUnlock()

	rels, ok := g.outgoing[nodeID]
	if !ok {
		return nil
	}

	// Find all CALLS relationships and get target nodes
	var callees []*GraphNode
	for _, rel := range rels {
		if rel.Type == RelCalls {
			if callee, exists := g.nodes[rel.Target]; exists {
				callees = append(callees, callee)
			}
		}
	}

	return callees
}

// Stats returns a summary of graph size.
func (g *KnowledgeGraph) Stats() map[string]int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return map[string]int{
		"nodes":         len(g.nodes),
		"relationships": len(g.relationships),
	}
}

// cascadeRelationshipsForNode removes all relationships where the node is source or target.
// Must be called with the write lock held.
func (g *KnowledgeGraph) cascadeRelationshipsForNode(nodeID string) {
	// Remove outgoing relationships
	outRels, ok := g.outgoing[nodeID]
	if ok {
		for _, rel := range outRels {
			delete(g.relationships, rel.ID)
			delete(g.byRelType[rel.Type], rel.ID)
			delete(g.incoming[rel.Target], rel.ID)
		}
		delete(g.outgoing, nodeID)
	}

	// Remove incoming relationships
	inRels, ok := g.incoming[nodeID]
	if ok {
		for _, rel := range inRels {
			delete(g.relationships, rel.ID)
			delete(g.byRelType[rel.Type], rel.ID)
			delete(g.outgoing[rel.Source], rel.ID)
		}
		delete(g.incoming, nodeID)
	}
}
