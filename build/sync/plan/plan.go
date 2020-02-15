// Package plan handles the synchronization plan.
//
// Each synchronization plan is a set of checks and actions to perform on specified paths
// that will result in the "plugin" repository being updated.
package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Plan defines the plan for synchronizing a plugin and a template directory.
type Plan struct {
	Checks []Check `json:"checks"`
	// Each path has multiple actions associated, each a fallback for the one
	// previous to it.
	Paths map[string][]Action
}

// UnmarshalJSON implements the `json.Unmarshaler` interface.
func (p *Plan) UnmarshalJSON(raw []byte) error {
	var t jsonPlan
	if err := json.Unmarshal(raw, &t); err != nil {
		return err
	}
	p.Checks = make([]Check, len(t.Checks))
	for i, check := range t.Checks {
		c, err := parseCheck(check.Type, check.Params)
		if err != nil {
			return fmt.Errorf("failed to parse check %q: %v", check.Type, err)
		}
		p.Checks[i] = c
	}

	if len(t.Paths) > 0 {
		p.Paths = make(map[string][]Action)
	}
	for _, path := range t.Paths {
		var err error
		pathActions := make([]Action, len(path.Actions))
		for i, action := range path.Actions {
			var actionConditions []Check
			if len(action.Conditions) > 0 {
				actionConditions = make([]Check, len(action.Conditions))
			}
			for j, check := range action.Conditions {
				actionConditions[j], err = parseCheck(check.Type, check.Params)
				if err != nil {
					return err
				}
			}
			pathActions[i], err = parseAction(action.Type, action.Params, actionConditions)
			if err != nil {
				return err
			}
		}
		p.Paths[path.Path] = pathActions
	}
	return nil
}

// Execute executes the synchronization plan.
func (p *Plan) Execute(c Setup) error {
	c.Logf("running pre-checks")
	for _, check := range p.Checks {
		err := check.Check("", c) // For pre-sync checks, the path is ignored.
		if err != nil {
			return fmt.Errorf("failed check: %v", err)
		}
	}
	result := make([]pathResult, 0, len(p.Paths))
	c.Logf("running actions")
PATHS_LOOP:
	for path, actions := range p.Paths {
		c.Logf("syncing path %q", path)
	ACTIONS_LOOP:
		for i, action := range actions {
			c.Logf("running action for path %q", path)
			err := action.Check(path, c)
			if IsCheckFail(err) {
				c.Logf("check failed, not running action: %v", err)
				// If a check for an action fails, we switch to
				// the next action associated with the path.
				if i == len(actions)-1 { // no actions to fallback to.
					c.Logf("path %q not handled - no more fallbacks", path)
					result = append(result,
						pathResult{
							Path:    path,
							Status:  statusFailed,
							Message: fmt.Sprintf("check failed, %s", err.Error()),
						})
				}
				continue ACTIONS_LOOP
			} else if err != nil {
				c.LogErrorf("unexpected error when running check: %v", err)
				return fmt.Errorf("failed to run checks for action: %v", err)
			}
			err = action.Run(path, c)
			if err != nil {
				c.LogErrorf("action failed: %v", err)
				return fmt.Errorf("action failed: %v", err)
			}
			c.Logf("path %q sync'ed succesfully", path)
			result = append(result,
				pathResult{
					Path:   path,
					Status: statusUpdated,
				})

			continue PATHS_LOOP
		}
	}

	// Print execution result.
	sort.SliceStable(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	for _, res := range result {
		if res.Message != "" {
			fmt.Fprintf(os.Stdout, "%s\t%s: %s\n", res.Status, res.Path, res.Message)
		} else {
			fmt.Fprintf(os.Stdout, "%s\t%s\n", res.Status, res.Path)
		}
	}
	return nil
}

// Check returns an error if the condition fails.
type Check interface {
	Check(string, Setup) error
}

// Action runs the defined action.
type Action interface {
	// Run performs the action on the specified path.
	Run(string, Setup) error
	// Check runs checks associated with the action
	// before running it.
	Check(string, Setup) error
}

// jsonPlan is used to unmarshal Plan structures.
type jsonPlan struct {
	Checks []struct {
		Type   string          `json:"type"`
		Params json.RawMessage `json:"params,omitempty"`
	}
	Paths []struct {
		Path    string `json:"path"`
		Actions []struct {
			Type       string          `json:"type"`
			Params     json.RawMessage `json:"params,omitempty"`
			Conditions []struct {
				Type   string          `json:"type"`
				Params json.RawMessage `json:"params"`
			}
		}
	}
}

func parseCheck(checkType string, rawParams json.RawMessage) (Check, error) {
	var c Check

	var params interface{}

	switch checkType {
	case "repo_is_clean":
		tc := RepoIsCleanChecker{}
		params = &tc.Params
		c = &tc
	case "exists":
		tc := PathExistsChecker{}
		params = &tc.Params
		c = &tc
	case "file_unaltered":
		tc := FileUnalteredChecker{}
		params = &tc.Params
		c = &tc
	default:
		return nil, fmt.Errorf("unknown checker type %q", checkType)
	}

	if len(rawParams) > 0 {
		err := json.Unmarshal(rawParams, params)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal params for %s: %v", checkType, err)
		}
	}
	return c, nil
}

func parseAction(actionType string, rawParams json.RawMessage, checks []Check) (Action, error) {
	var a Action

	var params interface{}

	switch actionType {
	case "overwrite_file":
		ta := OverwriteFileAction{}
		ta.Conditions = checks
		params = &ta.Params
		a = &ta
	case "overwrite_directory":
		ta := OverwriteDirectoryAction{}
		ta.Conditions = checks
		params = &ta.Params
		a = &ta
	default:
		return nil, fmt.Errorf("unknown action type %q", actionType)
	}

	if len(rawParams) > 0 {
		err := json.Unmarshal(rawParams, params)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal params for %s: %v", actionType, err)
		}
	}
	return a, nil
}

// pathResult contains the result of synchronizing a path.
type pathResult struct {
	Path    string
	Status  status
	Message string
}

type status string

const (
	statusUpdated status = "UPDATED"
	statusFailed  status = "FAILED"
	statusInSync  status = "UP-TO-DATE"
)
