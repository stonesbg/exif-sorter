package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/barasher/go-exiftool"
	"github.com/charmbracelet/log"
	"github.com/pkg/errors"
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
	dateRegex := regexp.MustCompile(`(\d{8}(?:-\d{6})?)`)

	log.Infof("Carrying out the copy: %v", *copyFlag)

	// Traverse source directory and process each file
	filepath.Walk(*srcDirPtr, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Errorf("Error while accessing %q: %v", path, err)
			return nil
		}

		// Only process files with .jpg or .mp4 extensions
		if info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".jpg") && !strings.HasSuffix(strings.ToLower(info.Name()), ".mp4") {
			return nil
		}

		// Extract date from EXIF data or filename
		date, ifExif, err := extractDate(path, dateRegex)
		if err != nil {
			log.Errorf("Error while extracting date from %q: %+v", path, err)
			return nil
		}

		// Generate new file name with date
		newName := filepath.Join(*destDirPtr, date.Format(*folderFormat), filepath.Base(path))

		// Move or copy file
		if *copyFlag {
			err = copyFile(path, newName)
		} else {
			err = renameFile(path, newName)
		}
		if err != nil {
			log.Errorf("Error while processing %q: %+v", path, err)
			return nil
		}

		// Update EXIF data if requested
		if *updateExifFlag && !ifExif {
			log.Warnf("Need to update EXIF data of %q", newName)
			err = updateExif(newName, date)
			if err != nil {
				log.Errorf("Error while updating EXIF data of %q: %v", newName, err)
				return nil
			}
		}

		// Log file move or copy
		if *logFlag {
			log.Infof("%s %q -> %q", getActionString(*copyFlag), path, newName)
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
	et, err := exiftool.NewExiftool()
	if err != nil {
		return time.Time{}, false, errors.Errorf("Error when intializing: %v", err)
	}
	defer et.Close()

	fileInfos := et.ExtractMetadata(path)

	for _, fileInfo := range fileInfos {
		if fileInfo.Err != nil {
			log.Errorf("Error concerning %v: %v", fileInfo.File, fileInfo.Err)
			continue
		}

		for k, v := range fileInfo.Fields {
			log.Debugf("[%v] %v", k, v)
		}
	}

	dateStr := ""
	if strings.ToLower(filepath.Ext(path)) == ".mp4" {
		dateStr, _ = fileInfos[0].GetString("MediaCreateDate")
	} else if strings.ToLower(filepath.Ext(path)) == ".jpg" || strings.ToLower(filepath.Ext(path)) == ".jpeg" {
		dateStr, _ = fileInfos[0].GetString("DateTimeOriginal")
	}
	date, _ := time.Parse("2006:01:02 15:04:05", dateStr)
	if !date.IsZero() {
		return date, true, nil
	}

	// Extract date from filename
	matches := dateRegex.FindStringSubmatch(path)
	if len(matches) == 0 {
		return time.Time{}, false, errors.Errorf("unable to extract date from filename")
	}
	dateStr = matches[0]

	// Parse date string and return as time.Time
	if strings.Contains(dateStr, "-") {
		date, err = time.Parse("20060102-150405", dateStr)
	} else if strings.Contains(dateStr, "_") {
		date, err = time.Parse("20060102_150405", dateStr)
	} else {
		date, err = time.Parse("20060102", dateStr)
	}
	if err != nil {
		return time.Time{}, false, errors.WithStack(err)
	}
	return date, false, nil
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

func renameFile(src, dest string) error {
	err := ensureDir(dest)
	if err != nil {
		return err
	}

	// Create destination file for writing
	err = os.Rename(src, dest)
	if err != nil {
		return err
	}

	return nil
}

func updateExif(path string, date time.Time) error {
	e, err := exiftool.NewExiftool()
	if err != nil {
		log.Errorf("Error when intializing: %v", err)
		return err
	}
	defer e.Close()

	fileInfos := e.ExtractMetadata(path)

	dateStr := ""
	if strings.ToLower(filepath.Ext(path)) == ".mp4" {
		dateStr, _ = fileInfos[0].GetString("MediaCreateDate")
		log.Infof("Date Original %v changed to %v", dateStr, date.Format("2006-01-02 15:04:05"))
		fileInfos[0].SetString("MediaCreateDate", date.Format("2006-01-02 15:04:05"))
		fileInfos[0].SetString("CreateDate", date.Format("2006-01-02 15:04:05"))

	} else if strings.ToLower(filepath.Ext(path)) == ".jpg" || strings.ToLower(filepath.Ext(path)) == ".jepg" {
		dateStr, _ = fileInfos[0].GetString("DateTaken")
		log.Infof("Date Original %v changed to %v", dateStr, date.Format("2006-01-02 15:04:05"))

		fileInfos[0].SetString("DateTimeOriginal", date.Format("2006-01-02 15:04:05"))
		fileInfos[0].SetString("CreateDate", date.Format("2006-01-02 15:04:05"))
	}

	e.WriteMetadata(fileInfos)

	return nil
}

func getActionString(copyFlag bool) string {
	if copyFlag {
		return "Copied"
	}
	return "Moved"
}
