package objectdefs

import "testing"

func TestResolveAppearanceResource_FallbackToBase(t *testing.T) {
	def := &ObjectDef{
		Resource: "obj/box/box.png",
		Appearance: []Appearance{
			{
				ID: "full",
				When: &AppearanceWhen{
					Flags: []string{"container.has_items"},
				},
				Resource: "obj/box/box_open.png",
			},
		},
	}

	got := ResolveAppearanceResource(def, nil)
	if got != "obj/box/box.png" {
		t.Fatalf("unexpected resource: got %q want %q", got, "obj/box/box.png")
	}
}

func TestResolveAppearanceResource_FirstMatchWins(t *testing.T) {
	def := &ObjectDef{
		Resource: "base",
		Appearance: []Appearance{
			{
				ID: "first",
				When: &AppearanceWhen{
					Flags: []string{"f1"},
				},
				Resource: "r1",
			},
			{
				ID: "second",
				When: &AppearanceWhen{
					Flags: []string{"f1"},
				},
				Resource: "r2",
			},
		},
	}

	got := ResolveAppearanceResource(def, []string{"f1"})
	if got != "r1" {
		t.Fatalf("unexpected resource: got %q want %q", got, "r1")
	}
}

func TestResolveAppearanceResource_AllFlagsRequired(t *testing.T) {
	def := &ObjectDef{
		Resource: "base",
		Appearance: []Appearance{
			{
				ID: "combined",
				When: &AppearanceWhen{
					Flags: []string{"f1", "f2"},
				},
				Resource: "combo",
			},
		},
	}

	got := ResolveAppearanceResource(def, []string{"f1"})
	if got != "base" {
		t.Fatalf("unexpected resource: got %q want %q", got, "base")
	}

	got = ResolveAppearanceResource(def, []string{"f1", "f2"})
	if got != "combo" {
		t.Fatalf("unexpected resource: got %q want %q", got, "combo")
	}
}
