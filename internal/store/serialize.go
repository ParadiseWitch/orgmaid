package store

import (
	"bytes"
	"fmt"
	"strings"
)

// Serialize renders the journal back to org-mode. Days are written in the order
// they are held, which Parse and Save both keep ascending.
func (j Journal) Serialize() []byte {
	var b bytes.Buffer

	for i, day := range j.Days {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "* %s\n", day.Date)

		for _, item := range day.Items {
			b.WriteString("** ")
			if item.Todo != "" {
				b.WriteString(item.Todo)
				b.WriteByte(' ')
			}
			b.WriteString(item.Content)
			if len(item.Tags) > 0 {
				b.WriteString("  :")
				b.WriteString(strings.Join(item.Tags, ":"))
				b.WriteString(":")
			}
			b.WriteByte('\n')
			// SCHEDULED
			if item.Scheduled != nil {
				b.WriteString("   SCHEDULED: <")
				b.WriteString(item.Scheduled.String())
				b.WriteString(">\n")
			}
			// DEADLINE
			if item.Deadline != nil {
				b.WriteString("   DEADLINE: <")
				b.WriteString(item.Deadline.String())
				b.WriteString(">\n")
			}
			// CLOCK
			if item.Start != nil {
				b.WriteString("   CLOCK: [")
				b.WriteString(item.Start.String())
				b.WriteString("]")
				if item.End != nil {
					b.WriteString("--[")
					b.WriteString(item.End.String())
					b.WriteString("]")
				}
				b.WriteByte('\n')
			}
		}
	}

	return b.Bytes()
}
