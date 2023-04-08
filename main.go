package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/barasher/go-exiftool"
	"github.com/charmbracelet/log"
	"github.com/pkg/errors"
	"github.com/rwcarlsen/goexif/exif"
)

func main() {
	// Define command-line flags
	srcDirPtr := flag.String("src", "", "source directory")
	destDirPtr := flag.String("dest", "", "destination directory")
	copyFlag := flag.Bool("copy", false, "copy files instead of moving them")
	folderFormat := flag.String("datefmt", "2006/01/02", "date format to use for organizing files (default is YYYY/MM/DD)")
	updateExifFlag := flag.Bool("update-exif", false, "update EXIF data based on file name if no EXIF data is available")
	logFlag := flag.Bool("log", false, "enable logging")
	flag.Parse()

	// Check if required flags are provided
	if *srcDirPtr == "" || *destDirPtr == "" {
		log.Error("Please provide source and destination directories")
		os.Exit(1)
	}

	// Compile regex to extract date from filename
	dateRegex := regexp.MustCompile(`(\d{4})[\-_]?(\d{2})[\-_]?(\d{2})`)

	log.Infof("Carrying out the copy: %v", *copyFlag)

	// Traverse source directory and process each file
	filepath.Walk(*srcDirPtr, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Errorf("Error while accessing %q: %v\n", path, err)
			return nil
		}

		// Only process files with .jpg or .mp4 extensions
		if info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".jpg") && !strings.HasSuffix(strings.ToLower(info.Name()), ".mp4") {
			return nil
		}

		// Extract date from EXIF data or filename
		date, ifExif, err := extractDate(path, dateRegex)
		if err != nil {
			log.Errorf("Error while extracting date from %q: %+v\n", path, err)
			return nil
		}

		// Generate new file name with date
		newName := filepath.Join(*destDirPtr, date.Format(*folderFormat), filepath.Base(path))

		// Move or copy file
		if *copyFlag {
			err = copyFile(path, newName)
		} else {
			err = os.Rename(path, newName)
		}
		if err != nil {
			log.Errorf("Error while processing %q: %+v\n", path, err)
			return nil
		}

		// Update EXIF data if requested
		if *updateExifFlag && !ifExif {
			log.Warnf("Need to update EXIF data of %q\n", newName)
			err = updateExif(newName, date)
			if err != nil {
				log.Errorf("Error while updating EXIF data of %q: %v\n", newName, err)
				return nil
			}
		}

		// Log file move or copy
		if *logFlag {
			log.Infof("%s %q -> %q\n", getActionString(*copyFlag), path, newName)
		}

		return nil
	})
}

func ensureDir(path string) error {
	exPath := filepath.Dir(path)
	err := os.MkdirAll(exPath, os.ModePerm)
	if err != nil {
		log.Error(err)
	}
	return err
}

func extractDate(path string, dateRegex *regexp.Regexp) (time.Time, bool, error) {
	// Extract date from EXIF data
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, false, errors.WithStack(err)
	}
	defer f.Close()

	x, err := exif.Decode(f)
	if err == nil {
		date, _ := x.DateTime()
		if !date.IsZero() {
			return date, true, nil
		}
		f.Seek(0, 0)

		// Extract date from filename
		matches := dateRegex.FindStringSubmatch(path)
		if len(matches) != 2 {
			return time.Time{}, false, errors.Errorf("unable to extract date from filename")
		}
		dateStr := matches[1]

		// Parse date string and return as time.Time
		if strings.Contains(dateStr, "-") {
			date, err = time.Parse("20060102-150405", dateStr)
		} else {
			date, err = time.Parse("20060102_150405", dateStr)
		}
		if err != nil {
			return time.Time{}, false, errors.WithStack(err)
		}
		return date, false, nil
	}

	return time.Time{}, false, errors.WithStack(err)
}

func copyFile(src, dest string) error {
	// Open source file for reading
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	err = ensureDir(dest)
	if err != nil {
		return err
	}

	// Create destination file for writing
	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Copy file contents
	_, err = io.Copy(destFile, srcFile)
	return err
}

func updateExif(path string, date time.Time) error {
	e, err := exiftool.NewExiftool()
	if err != nil {
		log.Errorf("Error when intializing: %v\n", err)
		return err
	}
	defer e.Close()

	fileInfos := e.ExtractMetadata(path)

	for _, fileInfo := range fileInfos {
		if fileInfo.Err != nil {
			fmt.Printf("Error concerning %v: %v\n", fileInfo.File, fileInfo.Err)
			continue
		}

		for k, v := range fileInfo.Fields {
			log.Infof("[%v] %v\n", k, v)
		}
	}

	//fileInfos[0].SetString("Date Taken", "newTitle")
	//e.WriteMetadata(fileInfos)

	return nil
}

func getActionString(copyFlag bool) string {
	if copyFlag {
		return "Copied"
	}
	return "Moved"
}
