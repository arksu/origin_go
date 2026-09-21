package inventory

import (
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	constt "origin/internal/const"
	"origin/internal/craftdefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/types"
	"testing"
)

func mappedInventoryRecipe() *craftdefs.CraftDef {
	return &craftdefs.CraftDef{Inputs: []craftdefs.CraftInput{{ItemTag: "raw_meat", Count: 1, QualityWeight: 1}}, Outputs: []craftdefs.CraftOutput{{ItemKey: "roasted_meat", Count: 5}}, OutputByInputKey: map[string]string{"beef": "roasted_beef", "raw_pork": "roast_pork"}}
}

func setMappedInventoryRegistry(t *testing.T) {
	t.Helper()
	previous := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 1, Key: "beef", Tags: []string{"raw_meat"}, Size: itemdefs.Size{W: 1, H: 1}},
		{DefID: 2, Key: "raw_pork", Tags: []string{"raw_meat"}, Size: itemdefs.Size{W: 1, H: 1}},
		{DefID: 3, Key: "roasted_beef", Size: itemdefs.Size{W: 1, H: 1}},
		{DefID: 4, Key: "roast_pork", Size: itemdefs.Size{W: 1, H: 1}},
		{DefID: 5, Key: "roasted_meat", Size: itemdefs.Size{W: 1, H: 1}},
	}))
}

// craftFit mirrors the runtime fit check: resolve outputs from a fresh input preview, then simulate placement.
func craftFit(t *testing.T, executor *InventoryExecutor, world *ecs.World, playerID types.EntityID, player types.Handle, recipe *craftdefs.CraftDef, quality uint32) bool {
	t.Helper()
	outputs, err := ResolveCraftOutputs(recipe, executor.PreviewCraftInputs(world, playerID, player, recipe))
	if err != nil {
		return false
	}
	return executor.CanFitResolvedCraftOutputs(world, playerID, player, outputs, quality)
}

func TestMappedCraftInputOrderAndCommit(t *testing.T) {
	setMappedInventoryRegistry(t)
	world, playerID, player := setupTestWorld(t)
	rootFirst := createGridContainer(world, playerID, 0, 3, 1)
	rootSecond := createGridContainer(world, playerID, 1, 3, 1)
	nested := createGridContainer(world, 5000, 0, 3, 1)
	hand := createHandContainer(world, playerID, 0)
	ecs.AddComponent(world, player, components.InventoryOwner{Inventories: []components.InventoryLink{
		{Kind: constt.InventoryHand, OwnerID: playerID, Handle: hand},
		{Kind: constt.InventoryGrid, OwnerID: 5000, Handle: nested},
		{Kind: constt.InventoryGrid, OwnerID: playerID, Handle: rootFirst},
		{Kind: constt.InventoryGrid, OwnerID: playerID, Key: 1, Handle: rootSecond},
	}})
	for index, handle := range []types.Handle{rootFirst, rootSecond, nested, hand} {
		addItemToContainer(world, handle, components.InvItem{ItemID: types.EntityID(600 + index), TypeID: 1, Quantity: 1, Quality: uint32(30 + index), W: 1, H: 1})
	}
	// Slice order wins over coordinates and item IDs, and one stack unit is consumed.
	addItemToContainer(world, rootFirst, components.InvItem{ItemID: 100, TypeID: 2, Quantity: 2, Quality: 77, W: 1, H: 1, X: 1})
	executor := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
	recipe := mappedInventoryRecipe()
	for _, expected := range []struct {
		id      types.EntityID
		key     string
		quality uint64
	}{
		{600, "beef", 30}, {100, "raw_pork", 77}, {100, "raw_pork", 77}, {601, "beef", 31}, {602, "beef", 32}, {603, "beef", 33},
	} {
		preview := executor.PreviewCraftInputs(world, playerID, player, recipe)
		require.True(t, preview.Success)
		require.Equal(t, expected.id, preview.SourceItemID)
		require.Equal(t, expected.key, preview.SourceItemKey)
		require.Equal(t, expected.quality, preview.QualityWeighted)
		require.Equal(t, uint64(1), preview.QualityWeightSum)
		again := executor.PreviewCraftInputs(world, playerID, player, recipe)
		require.Equal(t, preview.SourceItemID, again.SourceItemID, "preview must not consume")
		outputs, err := ResolveCraftOutputs(recipe, preview)
		require.NoError(t, err)
		require.Equal(t, []craftdefs.CraftOutput{{ItemKey: recipe.OutputByInputKey[expected.key], Count: 1}}, outputs)
		committed := executor.CommitPreparedCraftInputs(world, playerID, player, &preview)
		require.True(t, committed.Success)
		require.Equal(t, expected.id, committed.SourceItemID)
		require.False(t, executor.CommitPreparedCraftInputs(world, playerID, player, &preview).Success, "plan is single-use")
	}
	require.False(t, executor.PreviewCraftInputs(world, playerID, player, recipe).Success)
}

func TestMappedCraftMissingEntryDoesNotSkipFirstInput(t *testing.T) {
	setMappedInventoryRegistry(t)
	world, playerID, player := setupTestWorld(t)
	root := createGridContainer(world, playerID, 0, 3, 1)
	ecs.AddComponent(world, player, components.InventoryOwner{Inventories: []components.InventoryLink{{Kind: constt.InventoryGrid, OwnerID: playerID, Handle: root}}})
	addItemToContainer(world, root, components.InvItem{ItemID: 600, TypeID: 1, Quantity: 1, Quality: 11, W: 1, H: 1})
	addItemToContainer(world, root, components.InvItem{ItemID: 601, TypeID: 2, Quantity: 1, Quality: 22, W: 1, H: 1, X: 1})
	executor := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
	recipe := mappedInventoryRecipe()
	delete(recipe.OutputByInputKey, "beef")
	preview := executor.PreviewCraftInputs(world, playerID, player, recipe)
	require.True(t, preview.Success)
	_, err := ResolveCraftOutputs(recipe, preview)
	var missing *CraftMapEntryMissingError
	require.ErrorAs(t, err, &missing)
	require.EqualError(t, err, "no mapped output entry for source item beef")
	require.False(t, craftFit(t, executor, world, playerID, player, recipe, 11))
	container, _ := ecs.GetComponent[components.InventoryContainer](world, root)
	require.Len(t, container.Items, 2)
	recipe.OutputByInputKey = map[string]string{}
	_, err = ResolveCraftOutputs(recipe, preview)
	require.ErrorAs(t, err, &missing)
	recipe.OutputByInputKey = map[string]string{"beef": "unknown"}
	_, err = ResolveCraftOutputs(recipe, preview)
	require.ErrorContains(t, err, "target item unknown")
	recipe.OutputByInputKey = nil
	outputs, err := ResolveCraftOutputs(recipe, preview)
	require.NoError(t, err)
	require.Equal(t, recipe.Outputs, outputs)
}

func TestMappedCraftFitUsesTargetDefinition(t *testing.T) {
	for _, sourceKey := range []string{"beef", "raw_pork"} {
		for _, targetFits := range []bool{false, true} {
			t.Run(sourceKey+map[bool]string{true: " target fits", false: " preview fits"}[targetFits], func(t *testing.T) {
				setMappedInventoryRegistry(t)
				recipe := mappedInventoryRecipe()
				source, _ := itemdefs.Global().GetByKey(sourceKey)
				target, _ := itemdefs.Global().GetByKey(recipe.OutputByInputKey[sourceKey])
				preview, _ := itemdefs.Global().GetByKey("roasted_meat")
				if targetFits {
					preview.Size.W = 3
				} else {
					target.Size.W = 3
				}
				world, playerID, player := setupTestWorld(t)
				root := createGridContainer(world, playerID, 0, 2, 1)
				ecs.AddComponent(world, player, components.InventoryOwner{Inventories: []components.InventoryLink{{Kind: constt.InventoryGrid, OwnerID: playerID, Handle: root}}})
				addItemToContainer(world, root, components.InvItem{ItemID: 600, TypeID: uint32(source.DefID), Quantity: 1, Quality: 37, W: 1, H: 1})
				executor := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
				require.Equal(t, targetFits, craftFit(t, executor, world, playerID, player, recipe, 37))
				if targetFits {
					target.Allowed.Grid = boolPtr(false)
					require.False(t, craftFit(t, executor, world, playerID, player, recipe, 37), "mapped restrictions must be used")
					target.Allowed.Grid = boolPtr(true)
					ecs.MutateComponent[components.InventoryContainer](world, root, func(c *components.InventoryContainer) bool { c.Width = 1; return true })
					require.False(t, craftFit(t, executor, world, playerID, player, recipe, 37), "input's occupied cell is not free before consumption")
				}
			})
		}
	}
}

func TestPreparedCraftRejectsStaleContainer(t *testing.T) {
	setMappedInventoryRegistry(t)
	world, playerID, player := setupTestWorld(t)
	root := createGridContainer(world, playerID, 0, 3, 1)
	ecs.AddComponent(world, player, components.InventoryOwner{Inventories: []components.InventoryLink{{Kind: constt.InventoryGrid, OwnerID: playerID, Handle: root}}})
	addItemToContainer(world, root, components.InvItem{ItemID: 600, TypeID: 1, Quantity: 2, Quality: 37, W: 1, H: 1})
	executor := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
	preview := executor.PreviewCraftInputs(world, playerID, player, mappedInventoryRecipe())
	ecs.MutateComponent[components.InventoryContainer](world, root, func(c *components.InventoryContainer) bool { c.Version++; return true })
	require.False(t, executor.CommitPreparedCraftInputs(world, playerID, player, &preview).Success)
	container, _ := ecs.GetComponent[components.InventoryContainer](world, root)
	require.Equal(t, uint32(2), container.Items[0].Quantity)
}

func TestFixedCraftReservesExactInputsBeforeTags(t *testing.T) {
	setMappedInventoryRegistry(t)
	world, playerID, player := setupTestWorld(t)
	root := createGridContainer(world, playerID, 0, 3, 1)
	ecs.AddComponent(world, player, components.InventoryOwner{Inventories: []components.InventoryLink{{Kind: constt.InventoryGrid, OwnerID: playerID, Handle: root}}})
	addItemToContainer(world, root, components.InvItem{ItemID: 600, TypeID: 1, Quantity: 1, Quality: 10, W: 1, H: 1})
	addItemToContainer(world, root, components.InvItem{ItemID: 601, TypeID: 2, Quantity: 1, Quality: 30, W: 1, H: 1, X: 1})
	recipe := &craftdefs.CraftDef{Inputs: []craftdefs.CraftInput{{ItemTag: "raw_meat", Count: 1, QualityWeight: 1}, {ItemKey: "beef", Count: 1, QualityWeight: 2}}}
	executor := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
	prepared := executor.PreviewCraftInputs(world, playerID, player, recipe)
	require.True(t, prepared.Success)
	result := executor.CommitPreparedCraftInputs(world, playerID, player, &prepared)
	require.True(t, result.Success)
	require.Equal(t, uint64(50), result.QualityWeighted)
	require.Equal(t, uint64(3), result.QualityWeightSum)
	container, _ := ecs.GetComponent[components.InventoryContainer](world, root)
	require.Empty(t, container.Items)
}

func TestCraftConsumptionPreservesQualityOverflowFailure(t *testing.T) {
	setMappedInventoryRegistry(t)
	world, playerID, player := setupTestWorld(t)
	root := createGridContainer(world, playerID, 0, 3, 1)
	ecs.AddComponent(world, player, components.InventoryOwner{Inventories: []components.InventoryLink{{Kind: constt.InventoryGrid, OwnerID: playerID, Handle: root}}})
	const maximum = ^uint32(0)
	addItemToContainer(world, root, components.InvItem{ItemID: 600, TypeID: 1, Quantity: maximum, Quality: maximum, W: 1, H: 1})
	executor := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
	recipe := &craftdefs.CraftDef{Inputs: []craftdefs.CraftInput{{ItemTag: "raw_meat", Count: maximum, QualityWeight: maximum}}}
	result := executor.PreviewCraftInputs(world, playerID, player, recipe)
	require.False(t, result.Success)
	require.True(t, result.Overflow)
	container, _ := ecs.GetComponent[components.InventoryContainer](world, root)
	require.Equal(t, maximum, container.Items[0].Quantity)
}

func TestMappedCraftFitUsesNestedContentRules(t *testing.T) {
	for _, allowedKey := range []string{"roasted_beef", "roasted_meat"} {
		t.Run(allowedKey, func(t *testing.T) {
			setMappedInventoryRegistry(t)
			definitions := []itemdefs.ItemDef{}
			for _, definition := range itemdefs.Global().All() {
				definitions = append(definitions, *definition)
			}
			definitions = append(definitions, itemdefs.ItemDef{DefID: 200, Key: "bag", Name: "Bag", Size: itemdefs.Size{W: 1, H: 1}, Container: &itemdefs.ContainerDef{Size: itemdefs.Size{W: 1, H: 1}, Rules: itemdefs.ContentRules{AllowItemKeys: []string{allowedKey}}}})
			itemdefs.SetGlobalForTesting(itemdefs.NewRegistry(definitions))
			world, playerID, player, root, _, hand, _ := setupGiveItemWorld(t)
			addItemToContainer(world, root, components.InvItem{ItemID: 600, TypeID: 1, Quantity: 1, Quality: 37, W: 1, H: 1, X: 1})
			addItemToContainer(world, hand, components.InvItem{ItemID: 601, TypeID: 2, Quantity: 1, Quality: 11, W: 1, H: 1})
			executor := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
			require.Equal(t, allowedKey == "roasted_beef", craftFit(t, executor, world, playerID, player, mappedInventoryRecipe(), 37))
		})
	}
}
