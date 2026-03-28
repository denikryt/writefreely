/*
 * Copyright © 2026 Musing Studio LLC.
 *
 * This file is part of WriteFreely.
 *
 * WriteFreely is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License, included
 * in the LICENSE file in this source code package.
 */

package writefreely

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeTags(t *testing.T) {
	got := normalizeTags(" Linux,  Open Source, blender , linux, ,")
	want := []string{"linux", "open-source", "blender"}

	assert.Equal(t, want, got)
}
