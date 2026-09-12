package ui

import "testing"

func TestComputeLayout_DefaultWorkspaceGeometry(t *testing.T) {
	l := computeLayout(101, 28, focusSubscriptions)
	if l.leftWidth+l.rightWidth != l.width {
		t.Fatalf("column widths = %d + %d, want %d", l.leftWidth, l.rightWidth, l.width)
	}
	if l.metadata+l.domains+l.detail != l.height-2 {
		t.Fatalf("left heights = %d + %d + %d, want %d", l.metadata, l.domains, l.detail, l.height-2)
	}
	if l.logs+l.output != l.height-2 {
		t.Fatalf("right heights = %d + %d, want %d", l.logs, l.output, l.height-2)
	}
	if l.detail != (l.height-2)/2 || l.logs != l.output {
		t.Fatalf("default layout = %#v, want half-height Detail and equal Logs/Output", l)
	}
}

func TestComputeLayout_FocusedPanelExpands(t *testing.T) {
	for _, active := range []focus{focusWebsites, focusDetail, focusLogs, focusOutput} {
		l := computeLayout(100, 40, active)
		switch active {
		case focusWebsites:
			if l.domains <= l.detail {
				t.Fatalf("Domains should expand when focused: %#v", l)
			}
		case focusDetail:
			if l.detail <= l.domains {
				t.Fatalf("Detail should expand when focused: %#v", l)
			}
		case focusLogs:
			if l.logs <= l.output {
				t.Fatalf("Logs should expand when focused: %#v", l)
			}
		case focusOutput:
			if l.output <= l.logs {
				t.Fatalf("Output should expand when focused: %#v", l)
			}
		}
	}
}
