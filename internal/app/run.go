package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"maps"
	"mime"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
	"github.com/Gabes-Tint/mega-agents/internal/gitops"
	"github.com/Gabes-Tint/mega-agents/internal/router"
	"github.com/Gabes-Tint/mega-agents/internal/runs"
)

// runStepTimeout bounds one Git action so an unreachable remote cannot hold
// the request open indefinitely.
const runStepTimeout = 2 * time.Minute

const workspacePort = "workspace"

// Cancelled is the status of a run someone stopped.
const Cancelled = runs.Cancelled

var errNoStartingBlock = errors.New(
	"flag a GitHub block or an agent as the starting point to run the flow",
)

// gitActionLabels names each Git action a GitHub block can hold.
var gitActionLabels = map[string]string{
	"fetch":       "Fetch",
	"worktree":    "Create worktree",
	"rebase":      "Rebase",
	"issue":       "Read issue",
	"commit":      "Commit",
	"push":        "Push",
	"pullrequest": "Open pull request",
}

const (
	pullRequestPort = "pullRequest"
	issuePort       = "issue"
)

// Runs plans workflows and executes them as recorded runs: every run keeps
// its record and the log of every step in the store.
type Runs struct {
	Store *runs.Store
}

// Execution runs a started run to its end, calling observe (when not nil)
// with the record each time a step changes, and returns the finished record.
type Execution func(ctx context.Context, observe func(runs.Record)) runs.Record

// Start plans the flow and records a run of it with every step pending. It
// returns that record and the execution that runs it. A GitHub block
// flagged as a starting point runs its own actions from its starting action
// along their arrows, or a plain fetch when it holds no actions. Actions not
// reachable from a starting action do not run.
func (service Runs) Start(request WorkflowRequest) (runs.Record, Execution, error) {
	return service.start(request, nil)
}

var errNothingToRetry = errors.New("there is nothing to retry")

// Retry starts a new run of a failed, cancelled or interrupted run's
// workflow. Steps that succeeded before are not run again: they give their
// recorded outputs to the steps after them, so only what failed, and what
// depends on it, runs.
func (service Runs) Retry(id string) (runs.Record, Execution, error) {
	previous, err := service.Store.Load(id)
	if err != nil {
		return runs.Record{}, nil, err
	}
	switch previous.Status {
	case engine.Succeeded:
		return runs.Record{}, nil, fmt.Errorf("run %s succeeded; %w", id, errNothingToRetry)
	case engine.Running:
		return runs.Record{}, nil, fmt.Errorf("run %s is still running; %w yet", id, errNothingToRetry)
	}
	var request WorkflowRequest
	if err := json.Unmarshal(previous.Graph, &request); err != nil || len(previous.Graph) == 0 {
		return runs.Record{}, nil, fmt.Errorf("run %s did not record its workflow; %w", id, errNothingToRetry)
	}
	return service.start(request, &previous)
}

func (service Runs) start(request WorkflowRequest, previous *runs.Record) (runs.Record, Execution, error) {
	tasks, err := planRun(request)
	if err != nil {
		return runs.Record{}, nil, err
	}
	if err := engine.Validate(tasks); err != nil {
		return runs.Record{}, nil, err
	}
	if previous != nil {
		reuseSucceededSteps(tasks, *previous)
	}
	steps := engine.PendingSteps(tasks)
	name := request.Name
	if name == "" {
		name = "workflow"
	}
	record, err := service.Store.Create(name, steps)
	if err != nil {
		return runs.Record{}, nil, err
	}
	record.Graph, _ = json.Marshal(request)
	if previous != nil {
		record.RetryOf = previous.ID
	}
	if err := service.Store.Save(record); err != nil {
		return runs.Record{}, nil, err
	}
	execute := func(ctx context.Context, observe func(runs.Record)) runs.Record {
		return service.execute(ctx, record, tasks, observe)
	}
	return record, execute, nil
}

// reuseSucceededSteps replaces each task that succeeded in the previous run
// with one that gives back the outputs it recorded.
func reuseSucceededSteps(tasks []engine.Task, previous runs.Record) {
	succeeded := map[string]engine.Step{}
	for _, step := range previous.Steps {
		if step.Status == engine.Succeeded {
			succeeded[step.TaskID] = step
		}
	}
	for i, task := range tasks {
		step, ok := succeeded[task.ID]
		if !ok {
			continue
		}
		outputs := restoredOutputs(step.Outputs)
		details := maps.Clone(step.Details)
		if details == nil {
			details = map[string]any{}
		}
		details["reusedFrom"] = previous.ID
		name := task.Name
		tasks[i].Loop = nil
		tasks[i].Run = func(context.Context, []engine.Input, io.Writer) (engine.Result, error) {
			return engine.Result{Outputs: outputs, Details: details}, nil
		}
		tasks[i].Run = logged(tasks[i].Run, fmt.Sprintf("♻️ Reused the result of %s from run %s", name, previous.ID))
	}
}

func logged(run func(context.Context, []engine.Input, io.Writer) (engine.Result, error), line string) func(context.Context, []engine.Input, io.Writer) (engine.Result, error) {
	return func(ctx context.Context, inputs []engine.Input, log io.Writer) (engine.Result, error) {
		fmt.Fprintln(log, line)
		return run(ctx, inputs, log)
	}
}

// restoredOutputs turns recorded outputs back into what tasks exchange: a
// workspace is a gitops.Workspace again, and every other output stays JSON.
func restoredOutputs(recorded map[string]any) map[string]any {
	outputs := make(map[string]any, len(recorded))
	for port, value := range recorded {
		if port == workspacePort {
			var workspace gitops.Workspace
			encoded, _ := json.Marshal(value)
			if json.Unmarshal(encoded, &workspace) == nil {
				outputs[port] = workspace
				continue
			}
		}
		outputs[port] = value
	}
	return outputs
}

func (service Runs) execute(
	ctx context.Context, record runs.Record, tasks []engine.Task, observe func(runs.Record),
) runs.Record {
	var logs []io.Closer
	defer func() {
		for _, log := range logs {
			_ = log.Close()
		}
	}()
	save := func() {
		if err := service.Store.Save(record); err != nil {
			stdlog.Printf("run %s: %v", record.ID, err)
		}
		if observe != nil {
			observe(record)
		}
	}
	finished, err := engine.Execute(ctx, tasks, engine.Options{
		Observe: func(run engine.Run) {
			record.Steps = run.Steps
			save()
		},
		Log: func(taskID string) io.Writer {
			log, err := service.Store.OpenLog(record.ID, taskID)
			if err != nil {
				stdlog.Printf("run %s: %v", record.ID, err)
				return nil
			}
			logs = append(logs, log)
			return log
		},
	})
	if err != nil {
		// Planning already accepted these tasks, so this cannot happen; the
		// record still ends failed rather than running forever.
		finished.Status = engine.Failed
	}
	if finished.Steps != nil {
		record.Steps = finished.Steps
	}
	record.Status = finished.Status
	if errors.Is(ctx.Err(), context.Canceled) {
		record.Status = Cancelled
	}
	record.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	save()
	return record
}

type runPlanner struct {
	nodeByID   map[string]WorkflowNodeInput
	childrenOf map[string][]WorkflowNodeInput
	request    WorkflowRequest
	// included are the blocks this run executes.
	included map[string]bool
}

func planRun(request WorkflowRequest) ([]engine.Task, error) {
	tasks, failures, _ := plan(request)
	if len(failures) > 0 {
		return nil, failures[0].err
	}
	return tasks, nil
}

// planFailure is a reason the flow cannot run, with the block it concerns
// when there is one.
type planFailure struct {
	nodeID string
	err    error
}

// plan turns the graph into tasks, collecting every reason it cannot run
// instead of stopping at the first, in the order a run would meet them. It
// also returns the blocks the run reaches from its starting points.
func plan(request WorkflowRequest) ([]engine.Task, []planFailure, map[string]bool) {
	nodeByID, err := validateGraph(request)
	if err != nil {
		return nil, []planFailure{{err: err}}, nil
	}
	included := map[string]bool{}
	planner := runPlanner{nodeByID: nodeByID, childrenOf: map[string][]WorkflowNodeInput{}, request: request, included: included}
	for _, node := range request.Nodes {
		if node.ParentID != "" {
			planner.childrenOf[node.ParentID] = append(planner.childrenOf[node.ParentID], node)
		}
	}
	var tasks []engine.Task
	var failures []planFailure
	for _, node := range request.Nodes {
		switch {
		case node.Start && planner.loopOf(node).ID != "":
			failures = append(failures, planFailure{nodeID: node.ID, err: fmt.Errorf(
				"%s: blocks inside a loop start with the loop; clear its starting point", node.Name,
			)})
		case node.Type == "agent" && node.Start:
			planner.include(node.ID, included)
		case node.Type == "github" && node.Start:
			start, hasActions, err := planner.startingAction(node)
			if err != nil {
				failures = append(failures, planFailure{nodeID: node.ID, err: err})
				continue
			}
			if !hasActions {
				tasks = append(tasks, planner.fetchTask(node, node))
				continue
			}
			planner.include(start.ID, included)
		}
	}
	if len(tasks) == 0 && len(included) == 0 && len(failures) == 0 {
		return nil, []planFailure{{err: errNoStartingBlock}}, included
	}
	for _, node := range request.Nodes {
		if !included[node.ID] || planner.loopOf(node).ID != "" {
			continue
		}
		var task engine.Task
		switch node.Type {
		case "loop":
			task, err = planner.loopTask(node, &failures)
		case "agent":
			task, err = planner.agentTask(node)
		case "jsonschema":
			task, err = planner.schemaTask(node)
		case "router":
			task, err = planner.routerTask(node)
		case "command":
			task, err = planner.commandTask(node)
		default:
			task, err = planner.actionTask(node)
		}
		if err != nil {
			failures = append(failures, planFailure{nodeID: node.ID, err: err})
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks, failures, included
}

// startingAction finds the action a GitHub block's sequence starts from.
func (planner runPlanner) startingAction(github WorkflowNodeInput) (WorkflowNodeInput, bool, error) {
	var actions []WorkflowNodeInput
	for _, child := range planner.childrenOf[github.ID] {
		if child.Type == "action" {
			actions = append(actions, child)
		}
	}
	for _, action := range actions {
		if action.Start {
			return action, true, nil
		}
	}
	if len(actions) > 0 {
		return WorkflowNodeInput{}, true, fmt.Errorf("flag the first action of %s as its starting point", github.Name)
	}
	return WorkflowNodeInput{}, false, nil
}

// executableTypes are the blocks a run executes when a start reaches them.
var executableTypes = map[string]bool{"action": true, "agent": true, "jsonschema": true, "router": true, "command": true, "loop": true}

// include marks the node and every executable node its arrows reach, and
// the blocks inside a loop it reaches.
func (planner runPlanner) include(id string, included map[string]bool) {
	if included[id] || !executableTypes[planner.nodeByID[id].Type] {
		return
	}
	included[id] = true
	switch planner.nodeByID[id].Type {
	case "loop":
		for _, child := range planner.loopBody(id) {
			included[child.ID] = true
		}
	case "agent":
		for _, child := range planner.childrenOf[id] {
			if child.Type == "jsonschema" {
				planner.include(child.ID, included)
			}
		}
	}
	for _, edge := range planner.request.Edges {
		if edge.From == id {
			planner.include(edge.To, included)
		}
	}
}

// loopBody are the blocks a loop repeats: the blocks directly inside it and
// the schema checks inside its agents.
func (planner runPlanner) loopBody(id string) []WorkflowNodeInput {
	var body []WorkflowNodeInput
	for _, child := range planner.childrenOf[id] {
		if !executableTypes[child.Type] {
			continue
		}
		body = append(body, child)
		if child.Type == "agent" {
			for _, check := range planner.childrenOf[child.ID] {
				if check.Type == "jsonschema" {
					body = append(body, check)
				}
			}
		}
	}
	return body
}

// checkedAgent is the agent whose reply a schema block inside it checks, or
// no block.
func (planner runPlanner) checkedAgent(node WorkflowNodeInput) WorkflowNodeInput {
	if parent := planner.nodeByID[node.ParentID]; node.Type == "jsonschema" && parent.Type == "agent" {
		return parent
	}
	return WorkflowNodeInput{}
}

// loopOf is the loop a block sits directly inside, or no block. A schema
// block belongs to the loop of the agent it checks.
func (planner runPlanner) loopOf(node WorkflowNodeInput) WorkflowNodeInput {
	if agent := planner.checkedAgent(node); agent.ID != "" {
		node = agent
	}
	if parent := planner.nodeByID[node.ParentID]; parent.Type == "loop" {
		return parent
	}
	return WorkflowNodeInput{}
}

// arrowsInto are the arrows a block receives. A block inside a loop that no
// block beside it feeds starts each iteration, and receives the arrows into
// the loop.
func (planner runPlanner) arrowsInto(node WorkflowNodeInput) []WorkflowEdgeInput {
	if agent := planner.checkedAgent(node); agent.ID != "" {
		return []WorkflowEdgeInput{{From: agent.ID, To: node.ID}}
	}
	var arrows []WorkflowEdgeInput
	for _, edge := range planner.request.Edges {
		if edge.To == node.ID {
			arrows = append(arrows, edge)
		}
	}
	if loop := planner.loopOf(node); len(arrows) == 0 && loop.ID != "" {
		return planner.arrowsInto(loop)
	}
	return arrows
}

// needsWorkspace are the Git actions that work inside a workspace.
var needsWorkspace = map[string]bool{"rebase": true, "commit": true, "push": true, "pullrequest": true}

func (planner runPlanner) actionTask(node WorkflowNodeInput) (engine.Task, error) {
	if _, known := gitActionLabels[node.Action]; !known {
		return engine.Task{}, fmt.Errorf("unknown Git action %q on %s", node.Action, node.Name)
	}
	fail := func(format string, args ...any) (engine.Task, error) {
		return engine.Task{}, fmt.Errorf("%s: %s", node.Name, fmt.Sprintf(format, args...))
	}
	github := planner.nodeByID[node.ParentID]
	needs, err := planner.incomingNeeds(node)
	if err != nil {
		return engine.Task{}, err
	}
	sources := planner.sourcesOf(needs)
	if needsWorkspace[node.Action] && !sources.workspace {
		return engine.Task{}, fmt.Errorf(
			"%s needs a workspace; connect a %s action before it", node.Name, gitActionLabels["worktree"],
		)
	}
	for _, text := range []string{node.Message, node.Title, node.Body} {
		if err := checkTemplate(text, sources); err != nil {
			return fail("%v", err)
		}
	}
	if node.Action == "issue" && node.Issue < 0 {
		return fail("the issue number must be 0 for the next available issue, or a positive number")
	}
	if node.Action == "fetch" {
		task := planner.fetchTask(node, github)
		task.Needs = needs
		return task, nil
	}
	return engine.Task{
		ID: node.ID, Name: node.Name, Kind: node.Action, Needs: needs,
		Run: func(ctx context.Context, inputs []engine.Input, log io.Writer) (engine.Result, error) {
			ctx, cancel := context.WithTimeout(gitops.WithLog(ctx, log), runStepTimeout)
			defer cancel()
			workspace, _ := workspaceIn(inputs)
			networked := node.Action == "push" || node.Action == "pullrequest" || node.Action == "issue"
			if networked && !github.Authenticated {
				return engine.Result{}, errors.New(
					`check "Already authenticated" on this GitHub block; token authentication is not supported yet`,
				)
			}
			switch node.Action {
			case "rebase":
				return workspaceResult(gitops.Rebase(ctx, workspace, node.Onto))
			case "commit":
				message := renderTemplate(node.Message, inputs, sources)
				if strings.TrimSpace(node.Message) == "" {
					message = "Changes from Mega Agents run"
				}
				commit, err := gitops.Commit(ctx, workspace, message)
				result, _ := workspaceResult(workspace, err)
				if err == nil {
					result.Details["commit"] = commit
				}
				return result, err
			case "push":
				return workspaceResult(workspace, gitops.Push(ctx, workspace))
			case "pullrequest":
				pull, err := gitops.OpenPullRequest(ctx, workspace, gitops.PullRequest{
					Title: renderTemplate(node.Title, inputs, sources),
					Body:  renderTemplate(node.Body, inputs, sources), Base: node.Base,
				})
				if err != nil {
					return engine.Result{}, err
				}
				value := map[string]any{"number": pull.Number, "url": pull.URL, "base": pull.Base, "title": pull.Title}
				result, _ := workspaceResult(workspace, nil)
				result.Outputs[pullRequestPort] = value
				result.Details["url"], result.Details["number"] = pull.URL, pull.Number
				return result, nil
			case "issue":
				repository := github.Repository
				if strings.TrimSpace(repository) == "" {
					dir, err := projectDir(planner.nodeByID[github.ParentID])
					if err != nil {
						return engine.Result{}, err
					}
					detected, err := gitops.DetectGitHubRepository(ctx, dir)
					if err != nil {
						return engine.Result{}, err
					}
					repository = detected.Repository
				}
				number := node.Issue
				if number == 0 {
					picked, err := gitops.NextIssue(ctx, repository, node.ignoreLabels())
					if err != nil {
						return engine.Result{}, err
					}
					for _, skipped := range picked.Skipped {
						if skipped.Assignee != "" {
							fmt.Fprintf(log, "Skipped issue #%d, assigned to @%s\n", skipped.Number, skipped.Assignee)
						} else {
							fmt.Fprintf(log, "Skipped issue #%d, labeled %q\n", skipped.Number, skipped.Label)
						}
					}
					fmt.Fprintf(log, "📌 Picked the next available issue #%d\n", picked.Number)
					number = picked.Number
				}
				issue, err := gitops.ReadIssue(ctx, repository, number)
				if err != nil {
					return engine.Result{}, err
				}
				// An issue that carries a label to ignore fails the step so
				// nothing after it works on an issue meant to be left alone.
				if label := gitops.IgnoredLabel(issue, node.ignoreLabels()); label != "" {
					return engine.Result{}, fmt.Errorf("issue #%d is labeled %q, one of the labels to ignore", number, label)
				}
				return engine.Result{Outputs: map[string]any{issuePort: issue}, Details: map[string]any{"issue": issue}}, nil
			}
			dir, err := projectDir(planner.nodeByID[github.ParentID])
			if err != nil {
				return engine.Result{}, err
			}
			return workspaceResult(gitops.CreateWorktree(ctx, gitops.WorktreeRequest{
				Dir: dir, Repository: github.Repository, Branch: node.Branch,
				Base: node.Base, Path: node.WorktreePath,
			}))
		},
	}, nil
}

// incomingNeeds lists what a block receives from the executed blocks with
// arrows into it: each arrow's output, and the workspace a block passes on
// alongside its own output.
func (planner runPlanner) incomingNeeds(node WorkflowNodeInput) ([]engine.Need, error) {
	var needs []engine.Need
	for _, edge := range planner.arrowsInto(node) {
		if !planner.included[edge.From] {
			continue
		}
		port, err := planner.sourcePort(edge)
		if err != nil {
			return nil, err
		}
		needs = append(needs, engine.Need{TaskID: edge.From, Port: port})
		if port != workspacePort && planner.passesWorkspace(edge.From, map[string]bool{}) {
			needs = append(needs, engine.Need{TaskID: edge.From, Port: workspacePort})
		}
	}
	return needs, nil
}

// passesWorkspace reports whether a block outputs a workspace: the Git
// actions that prepare or keep one, and agents and commands that receive one.
func (planner runPlanner) passesWorkspace(id string, visiting map[string]bool) bool {
	node := planner.nodeByID[id]
	switch node.Type {
	case "action":
		return node.Action == "worktree" || needsWorkspace[node.Action]
	case "agent", "command", "loop", "router":
		if visiting[id] {
			return false
		}
		visiting[id] = true
		for _, edge := range planner.arrowsInto(node) {
			if planner.included[edge.From] && planner.passesWorkspace(edge.From, visiting) {
				return true
			}
		}
	}
	return false
}

// sourcePort names the output an arrow carries from its source block, or
// returns "" for an arrow that only orders the two blocks.
func (planner runPlanner) sourcePort(edge WorkflowEdgeInput) (string, error) {
	source := planner.nodeByID[edge.From]
	switch {
	case source.Type == "action" && source.Action == "pullrequest":
		return pullRequestPort, nil
	case source.Type == "action" && source.Action == "issue":
		return issuePort, nil
	case source.Type == "action" && (source.Action == "worktree" || needsWorkspace[source.Action]):
		return workspacePort, nil
	case source.Type == "agent":
		return resultPort, nil
	case source.Type == "loop":
		switch edge.FromPort {
		case "":
			return donePort, nil
		case donePort, exhaustedPort:
			return edge.FromPort, nil
		}
		return "", fmt.Errorf("%s has no output %q; use %s or %s", source.Name, edge.FromPort, donePort, exhaustedPort)
	case source.Type == "jsonschema":
		switch edge.FromPort {
		case "":
			return validPort, nil
		case validPort, invalidPort:
			return edge.FromPort, nil
		}
		return "", fmt.Errorf("%s has no output %q; use %s or %s", source.Name, edge.FromPort, validPort, invalidPort)
	case source.Type == "command":
		switch edge.FromPort {
		case "":
			return passedPort, nil
		case passedPort, failedPort:
			return edge.FromPort, nil
		}
		return "", fmt.Errorf("%s has no output %q; use %s or %s", source.Name, edge.FromPort, passedPort, failedPort)
	case source.Type == "router":
		routes := []string{}
		for _, routeCase := range source.Cases {
			routes = append(routes, routeCase.Name)
		}
		routes = append(routes, router.Default)
		if edge.FromPort == "" {
			return "", fmt.Errorf("choose which route the arrow from %s to %s takes", source.Name, planner.nodeByID[edge.To].Name)
		}
		if !slices.Contains(routes, edge.FromPort) {
			return "", fmt.Errorf("%s has no route %q; use %s", source.Name, edge.FromPort, joinChoices(routes))
		}
		return edge.FromPort, nil
	}
	return "", nil
}

// joinChoices spells a list as "a, b or c".
func joinChoices(choices []string) string {
	if len(choices) == 1 {
		return choices[0]
	}
	return strings.Join(choices[:len(choices)-1], ", ") + " or " + choices[len(choices)-1]
}

// valueNeeds collects the arrows into a block that takes exactly one value,
// such as an agent's result, and nothing else.
func (planner runPlanner) valueNeeds(node WorkflowNodeInput, verb string) ([]engine.Need, error) {
	incoming, err := planner.incomingNeeds(node)
	if err != nil {
		return nil, err
	}
	var needs []engine.Need
	for _, need := range incoming {
		if need.Port != workspacePort {
			needs = append(needs, need)
		}
	}
	if len(needs) != 1 || needs[0].Port == "" {
		return nil, fmt.Errorf("%s: %s one value; connect exactly one block to it", node.Name, verb)
	}
	return needs, nil
}

func workspaceResult(workspace gitops.Workspace, err error) (engine.Result, error) {
	if err != nil {
		return engine.Result{}, err
	}
	return engine.Result{
		Outputs: map[string]any{workspacePort: workspace},
		Details: map[string]any{
			"repository": workspace.Repository, "remote": workspace.Remote, "path": workspace.Path,
			"branch": workspace.Branch, "base": workspace.Base, "baseRevision": workspace.BaseRevision,
		},
	}, nil
}

// fetchTask fetches the repository of the GitHub block inside its project's
// clone. The task belongs to node, which is the GitHub block itself when it
// holds no actions.
func (planner runPlanner) fetchTask(node WorkflowNodeInput, github WorkflowNodeInput) engine.Task {
	return engine.Task{
		ID: node.ID, Name: node.Name, Kind: "fetch",
		Run: func(ctx context.Context, _ []engine.Input, log io.Writer) (engine.Result, error) {
			if !github.Authenticated {
				return engine.Result{}, errors.New(
					`check "Already authenticated" on this GitHub block; token authentication is not supported yet`,
				)
			}
			dir, err := projectDir(planner.nodeByID[github.ParentID])
			if err != nil {
				return engine.Result{}, err
			}
			ctx, cancel := context.WithTimeout(gitops.WithLog(ctx, log), runStepTimeout)
			defer cancel()
			result, err := gitops.Fetch(ctx, dir, github.Repository)
			if err != nil {
				return engine.Result{}, err
			}
			return engine.Result{Details: map[string]any{
				"repository": result.Repository, "remote": result.Remote, "output": result.Output,
			}}, nil
		},
	}
}

func projectDir(project WorkflowNodeInput) (string, error) {
	switch {
	case project.Path == "":
		return "", errors.New("set the path on the project that contains this GitHub block")
	case !filepath.IsAbs(project.Path):
		return "", fmt.Errorf("the project path %s must be absolute", project.Path)
	}
	return project.Path, nil
}

func registerRunHandler(mux *http.ServeMux, service Runs) {
	// Cancel functions of the runs this server is executing, by run id.
	var active sync.Map
	// launch executes a started run in the background, cancellable by id.
	launch := func(r *http.Request, record runs.Record, execute Execution) {
		ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
		active.Store(record.ID, cancel)
		go func() {
			defer active.Delete(record.ID)
			defer cancel()
			execute(ctx, nil)
		}()
	}

	// The editor asks for the repository when a GitHub block lands in a
	// project, so the field can be prefilled from the project's clone.
	mux.HandleFunc("GET /api/git/repository", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if !filepath.IsAbs(path) {
			http.Error(w, "the project path must be absolute", http.StatusNotFound)
			return
		}
		detected, err := gitops.DetectGitHubRepository(r.Context(), path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, detected)
	})

	// Starting a run answers at once with the recorded run; it executes in
	// the background, and the editor follows it through GET /api/runs/{id}.
	mux.HandleFunc("POST /api/runs", func(w http.ResponseWriter, r *http.Request) {
		// Runs execute local commands, so only JSON is accepted: browsers must
		// preflight it cross-origin, and this server never approves a preflight.
		mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaType != "application/json" {
			http.Error(w, "runs require a JSON request", http.StatusUnsupportedMediaType)
			return
		}
		var request WorkflowRequest
		body := http.MaxBytesReader(w, r.Body, workflowMaxBodyBytes)
		if err := json.NewDecoder(body).Decode(&request); err != nil {
			http.Error(w, "invalid workflow request", http.StatusBadRequest)
			return
		}
		record, execute, err := service.Start(request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		launch(r, record, execute)
		writeJSON(w, http.StatusAccepted, record)
	})

	mux.HandleFunc("POST /api/runs/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaType != "application/json" {
			http.Error(w, "retrying a run requires a JSON request", http.StatusUnsupportedMediaType)
			return
		}
		record, execute, err := service.Retry(r.PathValue("id"))
		switch {
		case errors.Is(err, runs.ErrNotFound):
			http.Error(w, "run not found", http.StatusNotFound)
			return
		case errors.Is(err, errNothingToRetry):
			http.Error(w, err.Error(), http.StatusConflict)
			return
		case err != nil:
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		launch(r, record, execute)
		writeJSON(w, http.StatusAccepted, record)
	})

	mux.HandleFunc("POST /api/runs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaType != "application/json" {
			http.Error(w, "cancelling a run requires a JSON request", http.StatusUnsupportedMediaType)
			return
		}
		id := r.PathValue("id")
		if cancel, running := active.Load(id); running {
			cancel.(context.CancelFunc)()
			writeJSON(w, http.StatusAccepted, map[string]string{"id": id, "status": "cancelling"})
			return
		}
		if _, err := service.Store.Load(id); err != nil {
			storeError(w, "run", err)
			return
		}
		http.Error(w, "run "+id+" is not running here", http.StatusConflict)
	})

	mux.HandleFunc("GET /api/runs", func(w http.ResponseWriter, _ *http.Request) {
		records, err := service.Store.List()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, records)
	})

	mux.HandleFunc("GET /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		record, err := service.Store.Load(r.PathValue("id"))
		if err != nil {
			storeError(w, "run", err)
			return
		}
		writeJSON(w, http.StatusOK, record)
	})

	mux.HandleFunc("GET /api/runs/{id}/logs/{step}", func(w http.ResponseWriter, r *http.Request) {
		text, err := service.Store.ReadLog(r.PathValue("id"), r.PathValue("step"))
		if err != nil {
			storeError(w, "log", err)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, text)
	})
}

func storeError(w http.ResponseWriter, what string, err error) {
	if errors.Is(err, runs.ErrNotFound) {
		http.Error(w, what+" not found", http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
