package afptoline

import (
	"fmt"
	"maps"
	"slices"

	"github.com/pratik-saptarshi/go-afp-line/afp"
)

// mergeMCFMappings folds an incoming MCF field's mapping into the page's
// accumulated mapping. Every LID in the incoming map — mapped (ByLID) or
// unsupported (Unsupported) — that already exists in the accumulated map's
// ByLID or Unsupported is a loud duplicate: the X'24' LID is defined as a
// unique identifier (MO:DCA Resource Local Identifier triplet, p. 368), and
// v1 applies the uniform duplicate rule from afp.ParseMCFMapping across
// fields, rejecting the MO:DCA p. 232 double-byte font exception (v1 does
// not implement double-byte fonts). Fresh LIDs are added in ascending LID
// order — map iteration is nondeterministic, and with several simultaneous
// accumulated-map collisions the reported LID must not vary run to run.
// Names go into ByLID, unsupported LIDs appended to Unsupported in incoming
// order, so group order is preserved across the merge. offset and fieldID
// attribute the error to the MCF field under visit.
func mergeMCFMappings(acc *afp.MCFMapping, incoming afp.MCFMapping, offset int, fieldID string) error {
	for _, lid := range slices.Sorted(maps.Keys(incoming.ByLID)) {
		if err := mergeMCFCheck(acc, lid, offset, fieldID); err != nil {
			return err
		}
		acc.ByLID[lid] = incoming.ByLID[lid]
	}
	for _, lid := range incoming.Unsupported {
		if err := mergeMCFCheck(acc, lid, offset, fieldID); err != nil {
			return err
		}
		acc.Unsupported = append(acc.Unsupported, lid)
	}
	return nil
}

// mergeMCFCheck reports a fail-closed duplicate for lid when it is already
// present anywhere in the accumulated map.
func mergeMCFCheck(acc *afp.MCFMapping, lid byte, offset int, fieldID string) error {
	if _, dup := acc.ByLID[lid]; dup {
		return duplicateMCFLIDError(lid, offset, fieldID)
	}
	if slices.Contains(acc.Unsupported, lid) {
		return duplicateMCFLIDError(lid, offset, fieldID)
	}
	return nil
}

// duplicateMCFLIDError builds the uniform duplicate-LID diagnostic shared
// with afp.ParseMCFMapping: fail-closed, naming the LID and the unsupported
// p. 232 double-byte font exception.
func duplicateMCFLIDError(lid byte, offset int, fieldID string) error {
	return afp.DecodeError{
		Kind:    afp.KindStructure,
		Offset:  offset,
		FieldID: fieldID,
		Reason:  fmt.Sprintf("duplicate MCF local font ID %d; v1 does not support the MO:DCA p. 232 double-byte font exception", lid),
	}
}
