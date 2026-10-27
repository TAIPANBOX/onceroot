package main

import "testing"

func TestAutoUsesTheDialogOnlyOnOmarchy(t *testing.T) {
	omarchy, plain := t.TempDir(), t.TempDir()+"/absent"
	cases := []struct {
		mode, dir    string
		dialog, lift bool
	}{
		{"auto", omarchy, true, true}, {"auto", plain, false, false},
		{"dialog", plain, true, false}, {"review", omarchy, false, true},
	}
	for _, c := range cases {
		if dialog, lift := desk(c.mode, c.dir); dialog != c.dialog || lift != c.lift {
			t.Errorf("%s with omarchy=%v: dialog %v lift %v", c.mode, c.dir == omarchy, dialog, lift)
		}
	}
}
