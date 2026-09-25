package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"example.com/krn/internal/workspace"
)

type File struct {
	Objective   string   `json:"objective,omitempty"`
	Constraints []string `json:"constraints,omitempty"`
	Proven      []string `json:"proven,omitempty"`
	Open        []string `json:"open,omitempty"`
	Negative    []string `json:"negative,omitempty"`
}

func Run(args []string) error {
	r, err := workspace.Discover()
	if err != nil {
		return err
	}
	p := filepath.Join(r.Private, "state.json")
	s := File{}
	if e := workspace.ReadJSON(p, &s); e != nil && !os.IsNotExist(e) {
		return e
	}
	if len(args) == 0 || args[0] == "show" {
		return workspace.PrintJSON(s)
	}
	if len(args) == 1 && args[0] == "clear" {
		if err := workspace.Ensure(r); err != nil {
			return err
		}
		return workspace.WriteJSON(p, File{})
	}
	if len(args) < 3 {
		return errors.New("state set objective TEXT | add constraint|proven|open|negative TEXT | clear")
	}
	op, field, val := args[0], args[1], strings.Join(args[2:], " ")
	if op == "set" && field == "objective" {
		s.Objective = val
	} else if op == "add" {
		switch field {
		case "constraint":
			s.Constraints = append(s.Constraints, val)
		case "proven":
			s.Proven = append(s.Proven, val)
		case "open":
			s.Open = append(s.Open, val)
		case "negative":
			s.Negative = append(s.Negative, val)
		default:
			return errors.New("unknown state field")
		}
	} else {
		return errors.New("unknown state operation")
	}
	if err := workspace.Ensure(r); err != nil {
		return err
	}
	return workspace.WriteJSON(p, s)
}
