package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

const previewMaxPresetBytes = 1 << 20

type savePresetRequest struct {
	renderRequest
	Path string `json:"path"`
}

func decodePreviewRequest(writer http.ResponseWriter, request *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, previewMaxPresetBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("expected a single JSON object")
	}
	return nil
}

func (server *previewServer) presetPath(value string) (string, error) {
	path := strings.TrimSpace(value)
	if path == "" {
		return "", errors.New("preset path must not be empty")
	}
	if filepath.IsAbs(path) {
		relative, err := filepath.Rel(server.presets.Name(), path)
		if err != nil || !filepath.IsLocal(relative) {
			return "", errors.New("preset must be inside the server's preset directory")
		}
		path = relative
	} else {
		for _, candidate := range []string{path, filepath.Join("..", path), filepath.Join("..", "..", path)} {
			absolute, err := filepath.Abs(candidate)
			if err != nil {
				return "", err
			}
			relative, err := filepath.Rel(server.presets.Name(), absolute)
			if err == nil && filepath.IsLocal(relative) {
				path = relative
				break
			}
		}
	}
	path = filepath.Clean(path)
	if !filepath.IsLocal(path) || (filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml") {
		return "", errors.New("preset must be a .yaml or .yml file inside the server's preset directory")
	}
	return filepath.ToSlash(path), nil
}

func (server *previewServer) loadPreset(value string) (MapgenOptions, []byte, string, error) {
	path, err := server.presetPath(value)
	if err != nil {
		return MapgenOptions{}, nil, "", err
	}
	file, err := server.presets.Open(path)
	if err != nil {
		return MapgenOptions{}, nil, "", fmt.Errorf("open preset %q: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return MapgenOptions{}, nil, "", err
	}
	if !info.Mode().IsRegular() || info.Size() > previewMaxPresetBytes {
		return MapgenOptions{}, nil, "", errors.New("preset must be a regular YAML file no larger than 1 MiB")
	}
	content, err := io.ReadAll(io.LimitReader(file, previewMaxPresetBytes+1))
	if err != nil {
		return MapgenOptions{}, nil, "", err
	}
	if len(content) > previewMaxPresetBytes {
		return MapgenOptions{}, nil, "", errors.New("preset exceeds 1 MiB")
	}
	opts, err := decodeMapgenOptions(content, path, DefaultMapgenOptions())
	if err == nil {
		err = opts.Validate()
	}
	return opts, content, path, err
}

func (server *previewServer) requestBase(preset string) (MapgenOptions, error) {
	if preset == "" {
		return server.base, nil
	}
	opts, _, _, err := server.loadPreset(preset)
	return opts, err
}

func validatePresetWriteRequest(request *http.Request) error {
	host := request.Host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	address := net.ParseIP(strings.Trim(host, "[]"))
	if host != "localhost" && (address == nil || !address.IsLoopback()) {
		return errors.New("preset saves require a localhost host")
	}
	if request.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return errors.New("cross-site preset saves are not allowed")
	}
	if origin := request.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme != "http" || parsed.Host != request.Host {
			return errors.New("preset saves require a same-origin request")
		}
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("preset saves require application/json")
	}
	return nil
}

func (server *previewServer) handleSavePreset(writer http.ResponseWriter, request *http.Request) {
	if err := validatePresetWriteRequest(request); err != nil {
		writeError(writer, http.StatusForbidden, err.Error())
		return
	}
	var payload savePresetRequest
	if err := decodePreviewRequest(writer, request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}
	path, err := server.presetPath(payload.Path)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	base, content, _, err := server.loadPreset(payload.Preset)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	opts, err := resolvePreviewOptions(payload.renderRequest, base)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := encodePreviewPreset(content, opts)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if err := server.writePreset(path, updated); err != nil {
		server.logger.Error("save preview preset failed", zap.String("preset", path), zap.Error(err))
		writeError(writer, http.StatusInternalServerError, fmt.Sprintf("save preset %q: %v", path, err))
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"preset": path})
}

func encodePreviewPreset(content []byte, opts MapgenOptions) ([]byte, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("preset must contain a YAML mapping")
	}
	updates := struct {
		World worldConfig  `yaml:"world"`
		River RiverOptions `yaml:"river"`
	}{worldConfig{ChunksX: opts.ChunksX, ChunksY: opts.ChunksY, Seed: opts.Seed, Threads: opts.Threads}, opts.River}
	var replacement yaml.Node
	if err := replacement.Encode(updates); err != nil {
		return nil, err
	}
	mergePreviewPresetNode(document.Content[0], &replacement)
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	reloaded, err := decodeMapgenOptions(output.Bytes(), "saved preset", DefaultMapgenOptions())
	if err == nil {
		err = reloaded.Validate()
	}
	return output.Bytes(), err
}

func mergePreviewPresetNode(existing, replacement *yaml.Node) {
	if existing.Kind != yaml.MappingNode || replacement.Kind != yaml.MappingNode {
		head, line, foot := existing.HeadComment, existing.LineComment, existing.FootComment
		*existing = *replacement
		existing.HeadComment, existing.LineComment, existing.FootComment = head, line, foot
		return
	}
	for index := 0; index < len(replacement.Content); index += 2 {
		key, value := replacement.Content[index], replacement.Content[index+1]
		var target *yaml.Node
		for position := 0; position < len(existing.Content); position += 2 {
			if existing.Content[position].Value == key.Value {
				target = existing.Content[position+1]
				break
			}
		}
		if target == nil {
			existing.Content = append(existing.Content, key, value)
		} else {
			mergePreviewPresetNode(target, value)
		}
	}
}

func (server *previewServer) writePreset(path string, content []byte) error {
	mode := os.FileMode(0o644)
	info, err := server.presets.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("save target must be a regular file, not a symlink or directory")
		}
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary := filepath.Join(filepath.Dir(path), ".preview-"+rand.Text()+".tmp")
	file, err := server.presets.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer func() {
		if err := server.presets.Remove(temporary); err != nil && !errors.Is(err, os.ErrNotExist) {
			server.logger.Warn("remove temporary preset failed", zap.Error(err))
		}
	}()
	_, writeErr := file.Write(content)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return server.presets.Rename(temporary, path)
}
