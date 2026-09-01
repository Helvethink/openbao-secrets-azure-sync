package watcher

import (
	"encoding/json"
	"log/slog"
	"strings"
)

type auditRecordParser struct {
	mounts             map[string]struct{}
	namespaces         map[string]struct{}
	watchAllMounts     bool
	watchAllNamespaces bool
	logger             *slog.Logger
}

// newAuditRecordParser builds a parser limited to the configured KV mounts.
func newAuditRecordParser(mounts []string, namespaces []string, logger *slog.Logger) auditRecordParser {
	mountSet := make(map[string]struct{}, len(mounts))
	watchAllMounts := false
	for _, mount := range mounts {
		mount = strings.Trim(mount, "/")
		if mount == "*" {
			watchAllMounts = true
			continue
		}
		if mount != "" {
			mountSet[mount] = struct{}{}
		}
	}
	namespaceSet := make(map[string]struct{}, len(namespaces))
	watchAllNamespaces := len(namespaces) == 0
	for _, namespace := range namespaces {
		namespace = strings.Trim(namespace, "/")
		if namespace == "*" {
			watchAllNamespaces = true
			continue
		}
		namespaceSet[namespace] = struct{}{}
	}
	return auditRecordParser{
		mounts:             mountSet,
		namespaces:         namespaceSet,
		watchAllMounts:     watchAllMounts,
		watchAllNamespaces: watchAllNamespaces,
		logger:             logger,
	}
}

// parseLine converts one JSON audit line into a sync event.
//
// Only request-side write operations are accepted. Response records and reads
// are ignored to avoid feedback loops from the controller's own OpenBao reads.
func (p auditRecordParser) parseLine(line string) (Event, bool) {
	var record struct {
		Type    string `json:"type"`
		Request struct {
			Operation string `json:"operation"`
			Path      string `json:"path"`
			Namespace struct {
				ID   string `json:"id"`
				Path string `json:"path"`
			} `json:"namespace"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &record); err != nil {
		p.logger.Debug("ignore non-json audit line")
		return Event{}, false
	}
	if record.Type != "request" {
		p.logger.Debug("ignore audit record type", "type", record.Type)
		return Event{}, false
	}
	if !isWriteOperation(record.Request.Operation) {
		p.logger.Debug("ignore audit operation",
			"operation", record.Request.Operation,
			"path", record.Request.Path,
		)
		return Event{}, false
	}
	if record.Request.Path == "" {
		p.logger.Debug("ignore audit record without request path")
		return Event{}, false
	}

	event, ok := p.parsePath(record.Request.Operation, record.Request.Path)
	if !ok {
		p.logger.Info("ignore audit path outside watched mounts",
			"operation", record.Request.Operation,
			"path", record.Request.Path,
		)
		return Event{}, false
	}
	if namespace := namespaceName(record.Request.Namespace.Path, record.Request.Namespace.ID); namespace != "" {
		event.Namespace = namespace
	}
	if !p.namespaceAllowed(event.Namespace) {
		p.logger.Info("ignore audit path outside watched namespaces",
			"namespace", event.Namespace,
			"operation", event.Operation,
			"mount", event.Mount,
			"path", event.Path,
		)
		return Event{}, false
	}
	p.logger.Info("accepted audit event",
		"namespace", event.Namespace,
		"mount", event.Mount,
		"path", event.Path,
		"operation", event.Operation,
	)
	return event, true
}

// parsePath extracts the KV v2 mount and logical secret path from an audit path.
func (p auditRecordParser) parsePath(operation, requestPath string) (Event, bool) {
	requestPath = strings.Trim(requestPath, "/")
	parts := strings.Split(requestPath, "/")
	if p.watchAllMounts {
		return parseWildcardMountPath(operation, parts)
	}
	for mount := range p.mounts {
		mountParts := strings.Split(mount, "/")
		for i := 0; i+len(mountParts) < len(parts); i++ {
			if !samePathParts(parts[i:i+len(mountParts)], mountParts) {
				continue
			}
			operationPart := parts[i+len(mountParts)]
			switch operationPart {
			case "data", "metadata", "delete", "undelete", "destroy":
				return Event{
					Mount:     strings.Join(mountParts, "/"),
					Path:      strings.Join(parts[i+len(mountParts)+1:], "/"),
					Namespace: strings.Join(parts[:i], "/"),
					Operation: operation,
				}, true
			}
		}
	}
	return Event{}, false
}

func parseWildcardMountPath(operation string, parts []string) (Event, bool) {
	for i, part := range parts {
		switch part {
		case "data", "metadata", "delete", "undelete", "destroy":
			if i == 0 || i+1 >= len(parts) {
				return Event{}, false
			}
			return Event{
				Mount:     parts[i-1],
				Path:      strings.Join(parts[i+1:], "/"),
				Namespace: strings.Join(parts[:i-1], "/"),
				Operation: operation,
			}, true
		}
	}
	return Event{}, false
}

func samePathParts(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func namespaceName(pathValue, id string) string {
	pathValue = strings.Trim(pathValue, "/")
	if pathValue != "" {
		return pathValue
	}
	if id == "" || id == "root" {
		return ""
	}
	return id
}

func (p auditRecordParser) namespaceAllowed(namespace string) bool {
	namespace = strings.Trim(namespace, "/")
	if p.watchAllNamespaces {
		return true
	}
	_, ok := p.namespaces[namespace]
	return ok
}

// isWriteOperation reports whether an OpenBao audit operation may require sync.
func isWriteOperation(operation string) bool {
	switch operation {
	case "create", "update", "delete":
		return true
	default:
		return false
	}
}
