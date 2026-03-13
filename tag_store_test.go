package writefreely

import (
	"reflect"
	"testing"
)

func TestNormalizeTags(t *testing.T) {
	got := normalizeTags(" Linux,  Open Source, blender , linux, ,")
	want := []string{"linux", "open-source", "blender"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeTags() = %#v, want %#v", got, want)
	}
}
