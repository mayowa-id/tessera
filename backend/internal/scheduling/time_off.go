package scheduling

// Block is a dated unavailability window or approved leave. Reasons stay private.
type Block struct {
	Interval
	Kind string
}

func CheckTimeOff(proposed Proposed, blocks []Block) []Violation {
	result := []Violation{}
	seen := map[string]bool{}
	for _, block := range blocks {
		if !proposed.Start.Before(block.End) || !block.Start.Before(proposed.End) || seen[block.Kind] {
			continue
		}
		var code, message string
		switch block.Kind {
		case "leave":
			code, message = "APPROVED_LEAVE", "The employee has approved leave during this shift."
		case "unavailable":
			code, message = "EMPLOYEE_UNAVAILABLE", "The employee is unavailable during this shift."
		default:
			continue
		}
		seen[block.Kind] = true
		result = append(result, Violation{EmployeeID: proposed.EmployeeID, ShiftID: proposed.ShiftID, Code: code, Severity: "hard", Message: message})
	}
	return result
}
