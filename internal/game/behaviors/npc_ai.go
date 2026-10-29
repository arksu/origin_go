package behaviors

import (
	"fmt"
	"math"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
	"origin/internal/types"
)

const npcAIKey = "npc_ai"

// npcAIBehavior has no actor state or world dependencies. The same immutable
// catalog is used by definition validation and each world's controller.
type npcAIBehavior struct{ catalog *NPCCatalog }

func NewNPCAIBehavior(catalog *NPCCatalog) contracts.Behavior { return npcAIBehavior{catalog: catalog} }
func (b npcAIBehavior) npcCatalog() *NPCCatalog {
	if b.catalog != nil {
		return b.catalog
	}
	return DefaultNPCCatalog()
}
func (npcAIBehavior) Key() string { return npcAIKey }
func (b npcAIBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil || ctx.Def == nil {
		return 0, fmt.Errorf("npc_ai requires an object definition")
	}
	def, ok := ctx.Def.(*objectdefs.ObjectDef)
	if !ok {
		return 0, fmt.Errorf("npc_ai requires an object definition")
	}
	var cfg contracts.NPCBehaviorConfig
	if err := decodeStrictJSON(ctx.RawConfig, &cfg); err != nil {
		return 0, fmt.Errorf("invalid npc_ai config: %w", err)
	}
	if err := b.npcCatalog().validate(&cfg); err != nil {
		return 0, err
	}
	if def.Static == nil || *def.Static || def.Components == nil || def.Components.Collider == nil {
		return 0, fmt.Errorf("npc_ai requires a dynamic object with a bounded collider")
	}
	box := def.Components.Collider
	if !npcFinitePositive(box.W) || !npcFinitePositive(box.H) || box.W > constt.CoordPerTile*4 || box.H > constt.CoordPerTile*4 {
		return 0, fmt.Errorf("npc_ai requires a dynamic object with a bounded collider")
	}
	if cfg.Priority <= 0 {
		cfg.Priority = defaultBehaviorPriority
	}
	def.SetNPCBehaviorConfig(cfg)
	return cfg.Priority, nil
}
func (npcAIBehavior) ApplyRuntime(*contracts.BehaviorRuntimeContext) contracts.BehaviorRuntimeResult {
	return contracts.BehaviorRuntimeResult{}
}
func (b npcAIBehavior) InitObject(ctx *contracts.BehaviorObjectInitContext) error {
	if ctx == nil {
		return nil
	}
	if _, ok := npcDefinition(ctx.World, ctx.Handle, ctx.EntityID, ctx.EntityType); !ok {
		return nil
	}
	c, ok := GetNPCController(ctx.World)
	if !ok {
		return fmt.Errorf("npc_ai requires an explicitly attached NPCController")
	}
	if c.catalog != b.npcCatalog() {
		return fmt.Errorf("npc_ai definition and controller catalogs differ")
	}
	return c.init(ctx)
}
func (b npcAIBehavior) OnScheduledTick(ctx *contracts.BehaviorTickContext) (contracts.BehaviorTickResult, error) {
	if ctx == nil {
		return contracts.BehaviorTickResult{}, nil
	}
	if _, ok := npcDefinition(ctx.World, ctx.Handle, ctx.EntityID, ctx.EntityType); !ok {
		return contracts.BehaviorTickResult{}, nil
	}
	c, ok := GetNPCController(ctx.World)
	if !ok {
		return contracts.BehaviorTickResult{}, fmt.Errorf("npc_ai requires an explicitly attached NPCController")
	}
	if c.catalog != b.npcCatalog() {
		return contracts.BehaviorTickResult{}, fmt.Errorf("npc_ai definition and controller catalogs differ")
	}
	return contracts.BehaviorTickResult{}, c.tick(ctx)
}
func npcDefinition(w *ecs.World, h types.Handle, id types.EntityID, entityType uint32) (*objectdefs.ObjectDef, bool) {
	if w == nil || !w.Alive(h) || ecs.ObjectDestructionPending(w, h) {
		return nil, false
	}
	actual, ok := w.GetExternalID(h)
	if !ok || actual != id {
		return nil, false
	}
	info, ok := ecs.GetComponent[components.EntityInfo](w, h)
	registry := objectdefs.Global()
	if !ok || info.TypeID != entityType || registry == nil {
		return nil, false
	}
	def, ok := registry.GetByID(int(entityType))
	return def, ok && def.NPCConfig != nil && def.HasBehavior(npcAIKey)
}
func npcTicksPerSecond(ts *ecs.TimeState) float64 {
	if ts.TickPeriod > 0 {
		return 1 / ts.TickPeriod.Seconds()
	}
	if ts.TickRate > 0 {
		return float64(ts.TickRate)
	}
	return 10
}
func npcValidPoint(p contracts.NPCPoint) bool {
	return !math.IsNaN(p.X) && !math.IsNaN(p.Y) && !math.IsInf(p.X, 0) && !math.IsInf(p.Y, 0) && math.Abs(p.X) <= 1<<30 && math.Abs(p.Y) <= 1<<30
}
