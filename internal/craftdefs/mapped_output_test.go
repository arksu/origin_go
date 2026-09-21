package craftdefs

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMappedOutputDefinitionLoading(t *testing.T) {
	setCraftDefsTestRegistries(t)
	for _, tc := range []struct {
		name, inputs, mapping, wantError string
		mapped                           bool
	}{
		{"absent", `[{"itemTag":"food","count":1,"qualityWeight":1}]`, ``, ``, false},
		{"empty", `[{"itemTag":"food","count":1,"qualityWeight":1}]`, `,"outputByInputKey":{}`, ``, true},
		{"valid incomplete generic tag", `[{"itemTag":"food","count":1,"qualityWeight":1}]`, `,"outputByInputKey":{"raw_meat":"cooked_meat"}`, ``, true},
		{"exact input", `[{"itemKey":"raw_meat","count":1,"qualityWeight":1}]`, `,"outputByInputKey":{}`, `exactly one input`, true},
		{"two rows", `[{"itemTag":"food","count":1,"qualityWeight":1},{"itemTag":"food","count":1,"qualityWeight":1}]`, `,"outputByInputKey":{}`, `exactly one input`, true},
		{"two units", `[{"itemTag":"food","count":2,"qualityWeight":1}]`, `,"outputByInputKey":{}`, `count 1`, true},
		{"empty tag", `[{"itemTag":" ","count":1,"qualityWeight":1}]`, `,"outputByInputKey":{}`, `exactly one of`, true},
		{"zero weight", `[{"itemTag":"food","count":1,"qualityWeight":0}]`, `,"outputByInputKey":{}`, `qualityWeight`, true},
		{"blank source", `[{"itemTag":"food","count":1,"qualityWeight":1}]`, `,"outputByInputKey":{" ":"cooked_meat"}`, `source key must not be empty`, true},
		{"blank target", `[{"itemTag":"food","count":1,"qualityWeight":1}]`, `,"outputByInputKey":{"raw_meat":" "}`, `target key`, true},
		{"unknown source", `[{"itemTag":"food","count":1,"qualityWeight":1}]`, `,"outputByInputKey":{"missing":"cooked_meat"}`, `unknown source item`, true},
		{"unknown target", `[{"itemTag":"food","count":1,"qualityWeight":1}]`, `,"outputByInputKey":{"raw_meat":"missing"}`, `unknown target item`, true},
		{"wrong tag", `[{"itemTag":"raw_meat","count":1,"qualityWeight":1}]`, `,"outputByInputKey":{"branch":"cooked_meat"}`, `does not have input tag raw_meat`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeCraftDefsTestFile(t, dir, "mapped.json", `{"v":1,"crafts":[{"defId":1,"key":"roast","ticksRequired":1,"inputs":`+tc.inputs+`,"outputs":[{"itemKey":"cooked_meat","count":5}]`+tc.mapping+`}]}`)
			registry, err := LoadFromDirectory(dir, craftDefsTestLogger())
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.ErrorContains(t, err, "key=roast")
				return
			}
			require.NoError(t, err)
			recipe, ok := registry.GetByKey("roast")
			require.True(t, ok)
			require.Equal(t, tc.mapped, recipe.OutputByInputKey != nil)
			require.Equal(t, uint32(5), recipe.Outputs[0].Count)
		})
	}
}

func TestMappedOutputEmptyMapDecoding(t *testing.T) {
	var recipe CraftDef
	require.NoError(t, json.Unmarshal([]byte(`{"outputByInputKey":{}}`), &recipe))
	require.NotNil(t, recipe.OutputByInputKey)
	encoded, err := json.Marshal(recipe)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"outputByInputKey":{}`)
	recipe.OutputByInputKey = nil
	encoded, err = json.Marshal(recipe)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"outputByInputKey"`)
}
