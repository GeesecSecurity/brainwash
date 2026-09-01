package slot

import "brainwash/internal/ir"

// ListMode controls how parsers enumerate sessions.
// cwd empty  → all projects under that agent home
// cwd set    → only sessions for that project
func ListFor(p Parser, cwd string) ([]ir.SessionRef, error) {
	return p.List(cwd)
}
