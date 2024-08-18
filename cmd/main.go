//go:build linux

package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

type auth struct {
	Token string `json:"token"`
}

type manifest struct {
	Config manifestConfig  `json:"config"`
	Layers []manifestLayer `json:"layers"`
}

type manifestConfig struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
}

type manifestLayer struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
}

// Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...
func main() {
	img := strings.Split(os.Args[2], ":")
	imgName := img[0]
	imgTag := "latest"
	if len(img) > 1 {
		imgTag = img[1]
	}
	command := os.Args[3]
	args := os.Args[4:len(os.Args)]

	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// creating a temporary directory that will act as the root filesystem for our containerized environment
	path, err := os.MkdirTemp("", "docker-*")
	if err != nil {
		panic("Failed to create dir: " + err.Error())
	}
	// ensure that temporary files do not persist on the host system after the containerized process has finished
	defer os.RemoveAll(path)

	// docker authentication dance
	res, err := http.Get(fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/%s:pull", imgName))
	if err != nil {
		panic("failed to get bearer token: " + err.Error())
	}
	defer res.Body.Close()

	data := &auth{}
	err = json.NewDecoder(res.Body).Decode(data)
	if err != nil {
		panic("failed to decode json: " + err.Error())
	}

	// fetch image manifest from docker
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://registry.hub.docker.com/v2/library/%s/manifests/%s", imgName, imgTag), nil)
	if err != nil {
		panic("failed to build manifest request: " + err.Error())
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", data.Token))
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")

	res, err = http.DefaultClient.Do(req)
	defer res.Body.Close()

	manifest := &manifest{}
	err = json.NewDecoder(res.Body).Decode(manifest)
	if err != nil {
		panic("failed to decode json: " + err.Error())
	}

	// pull layers of the image
	for _, layer := range manifest.Layers {
		req, err = http.NewRequest(http.MethodGet, fmt.Sprintf("https://registry.hub.docker.com/v2/library/%s/blobs/%s", imgName, layer.Digest), nil)
		if err != nil {
			panic("failed to build layer request: " + err.Error())
		}
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", data.Token))
		req.Header.Add("Accept", layer.MediaType)

		res, err = http.DefaultClient.Do(req)

		layerFile, err := gzip.NewReader(res.Body)
		if err != nil {
			panic("failed to uncompress layer file: " + err.Error())
		}

		tr := tar.NewReader(layerFile)

		for {
			header, err := tr.Next()

			if err == io.EOF {
				break
			}

			if err != nil {
				panic("failed to extract tar: " + err.Error())
			}

			newPath := filepath.Join(path, header.Name)

			switch header.Typeflag {
			case tar.TypeSymlink:
				fmt.Printf("creating symlink: %s\n", newPath)
				absolutePath := filepath.Join(path, header.Linkname)
				relativePath, err := filepath.Rel(filepath.Dir(newPath), absolutePath)
				if err != nil {
					panic("failed relative: " + err.Error())
				}
				err = os.Symlink(relativePath, newPath)
				if err != nil {
					panic("failed symlink: " + err.Error())
				}
			case tar.TypeDir:
				fmt.Printf("unpack dir: %s\n", newPath)
				if err := os.MkdirAll(newPath, header.FileInfo().Mode()); err != nil {
					panic("failed to mkdir: " + err.Error())
				}
			case tar.TypeReg:
				fmt.Printf("unpack reg: %s\n", newPath)
				file, err := os.Create(newPath)
				if err != nil {
					panic("failed to create: " + err.Error())
				}
				if _, err := io.Copy(file, tr); err != nil {
					panic("failed to copy: " + err.Error())
				}
				err = file.Close()
				if err != nil {
					panic("failed to close: " + err.Error())
				}
			}

			err = os.Chmod(newPath, header.FileInfo().Mode())
			if err != nil {
				if !os.IsNotExist(err) {
					panic("failed to chmod: " + err.Error())
				}
			}
		}

		err = res.Body.Close()
		if err != nil {
			panic("failed to close: " + err.Error())
		}
	}

	// locate the binary file for the command that we want to execute inside our container
	binPath, err := exec.LookPath(command)
	if err != nil {
		panic("Couldn't find binary: " + err.Error())
	}

	// creates a directory structure within the temporary directory to hold the binary that will be executed
	destPath := filepath.Join(path, binPath)
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		panic("Failed to create directories: " + err.Error())
	}

	// copy the binary that needs to be executed inside the new chroot environment
	err = copyFile(binPath, destPath)
	if err != nil {
		panic("Failed to copy: " + err.Error())
	}

	// change the root directory of the current process to the path
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Chroot:     path,
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID,
	}

	err = cmd.Run()

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}

		fmt.Printf("Err: %v", err)
		os.Exit(0)
	}
}

func copyFile(src, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return err
	}

	return destFile.Chmod(0755)
}
