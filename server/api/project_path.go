package api

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const managedProjectsRoot = "/poke/projects"

var projectNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

func validateProjectName(name string) error {
	if !projectNamePattern.MatchString(name) || name == "." || name == ".." {
		return fmt.Errorf("invalid project name")
	}
	return nil
}

func projectDirForName(name string) (string, error) {
	if err := validateProjectName(name); err != nil {
		return "", err
	}
	return filepath.Join(managedProjectsRoot, name), nil
}

func isPathWithinRoot(root, path string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func validateManagedPath(root, path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("project path must be absolute")
	}

	cleanPath := filepath.Clean(path)
	if !isPathWithinRoot(root, cleanPath) {
		return "", fmt.Errorf("project path is outside managed root")
	}

	info, err := os.Lstat(cleanPath)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("project path is not a regular directory")
	}

	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolvedPath, err := filepath.EvalSymlinks(cleanPath)
	if err != nil {
		return "", err
	}
	if !isPathWithinRoot(resolvedRoot, resolvedPath) {
		return "", fmt.Errorf("project path escapes managed root through a symbolic link")
	}

	return cleanPath, nil
}

func validateManagedProjectDir(path string) (string, error) {
	return validateManagedPath(managedProjectsRoot, path)
}

func isManagedProjectDir(path string) bool {
	_, err := validateManagedProjectDir(path)
	return err == nil
}
