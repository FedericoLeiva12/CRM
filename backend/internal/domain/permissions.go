package domain

// ValidatePermissions rejects ambiguous updates and grants that exceed their prerequisite.
func ValidatePermissions(permissions []Permission) error {
	seen := map[string]bool{}
	for _, permission := range permissions {
		if seen[permission.SectionID] {
			return Invalid("Duplicate section permission: " + permission.SectionID)
		}
		seen[permission.SectionID] = true
		if permission.Write && !permission.Read {
			return Invalid("Write access requires read access")
		}
		if permission.Delete && !permission.Write {
			return Invalid("Delete access requires write access")
		}
	}
	return nil
}
