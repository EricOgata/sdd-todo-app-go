package cli

import "github.com/spf13/cobra"

// Command group identifiers used by help output. Later features attach their
// commands by setting GroupID without restructuring the root.
const (
	// GroupTask groups capture, completion, listing, editing, overdue, and
	// search commands (F04–F10).
	GroupTask = "task"
	// GroupOrganization groups category and tag commands (F02, F03).
	GroupOrganization = "organization"
	// GroupMaintenance groups delete, restore, and deleted commands (F10).
	GroupMaintenance = "maintenance"
)

// commandGroups returns the documented groups in display order.
func commandGroups() []*cobra.Group {
	return []*cobra.Group{
		{ID: GroupTask, Title: "Task commands"},
		{ID: GroupOrganization, Title: "Organization commands"},
		{ID: GroupMaintenance, Title: "Maintenance commands"},
	}
}
