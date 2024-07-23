package main

import (
	"bufio"
	"compress/gzip"
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Version struct {
	date          string
	time          string
	label         string
	versionNumber int
	origFile      string
	file          string
	author        string
	id            string
}

type Archive struct {
	id   string
	file   string
}


func calculateSha1(file string) (string) {
	f, err := os.Open(file)
	if err != nil {
		die(err)
	}
	defer f.Close()

	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		die(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func die(err error) {
	fmt.Fprintf(os.Stderr, "Error: %s\n", err)
	os.Exit(1)
}


func readLabelsMap() map[string]string {
	labels := make(map[string]string)

	f, err := os.Open(LabelsTable)
	if err != nil {
		die(err)
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		field := strings.Fields(scanner.Text())
		labels[field[0]] = field[1]
	}
	return labels
}


func writeToLabelsMap(label, basename string) {
	f, err := os.OpenFile(LabelsTable, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		die(err)
	}
	defer f.Close()

	/* Labels-table has two columns: LABEL BASENAME */
	fmt.Fprintf(f, "%s %s\n", label, basename)
}


func readVersionsTable() (versionsList []*Version) {
	f, err := os.Open(VersionsTable)
	if err != nil {
		die(err)
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		v := new(Version)
		v.parse(scanner.Text())
		versionsList = append(versionsList, v)
	}
	if err := scanner.Err(); err != nil {
		die(err)
	}
	return 
}


func writeToVersionsTable(v Version) {
	f, err := os.OpenFile(VersionsTable, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		die(err)
	}

	/*
	 * Version entry order:
	 * DATE TIME LABEL VERSION ORIGFILE FILE AUTHOR ID
	 */

	fmt.Fprintf(f, "%s %s %s %d %s %s %s %s\n",
		v.date, v.time, v.label, v.versionNumber, v.origFile, v.file, v.author, v.id)
	f.Close()
}

func compress(inputFile, outputFile string) error {
	inFile, err := os.Open(inputFile)
	if err != nil {
		return err
	}
	defer inFile.Close()

	outFile, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer outFile.Close()

	gzipWriter := gzip.NewWriter(outFile)
	defer gzipWriter.Close()

	if _, err := io.Copy(gzipWriter, inFile); err != nil {
		return err
	}
	return nil
}

func decompress(inputFile string, outputFile string) error {
	inFile, err := os.Open(inputFile)
	if err != nil {
		return err
	}
	defer inFile.Close()

	outFile, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer outFile.Close()

	gzipReader, err := gzip.NewReader(inFile)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	if _, err := io.Copy(outFile, gzipReader); err != nil {
		return err
	}
	return nil
}

func getDate() string {
	date := time.Now()
	return date.Format("2006-01-02")
}

func getTime() string {
	/*
	 * This strange "15:04" is the golang way to
	 * say hour and minutes, zero-padded
	 */
	t := time.Now()
	return t.Format("15:04")
}


func printColumns(header string, file string) {
	cmd := exec.Command("column", "-t")
	cmd.Stdout = os.Stdout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		die(err)
	}
	defer stdin.Close()

	f, ferr := os.Open(file)
	if ferr != nil {
		die(err)
	}
	defer f.Close()

	if err := cmd.Start(); err != nil {
		die(err)
	}

	scanner := bufio.NewScanner(f)
	fmt.Fprintln(stdin, header)
	for scanner.Scan() {
		fmt.Fprintln(stdin, scanner.Text())
	}

	if err = scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading file:", err)
	}
	if err := stdin.Close(); err != nil {
		die(err)
	}
	if err := cmd.Wait(); err != nil {
		die(err)
	}
}

func askAuthorEmail() (email string) {
	fmt.Printf("Author email: ")
	_, err := fmt.Scan(&email)
	if err != nil {
		die(err)
	}
	return
}



func askConfirmation(label string, file string, email string) bool {
	fmt.Println()
	fmt.Printf("Label: %s\n", label)
	fmt.Printf("File : %s\n", file)
	fmt.Printf("Email: %s\n", email)
	fmt.Printf("Confirm update? (y/n): ")

	var ans string
	_, err := fmt.Scan(&ans)
	if err != nil {
		die(err)
	}

	if ans == "y" || ans == "yes" {
		return true
	} else {
		return false
	}
}



func getLastVersionNumber(label string) (lastVersion int) {
	versionsTable := readVersionsTable()
	for _, v := range versionsTable {
		if v.label == "main" {
			lastVersion = v.versionNumber
		}
	}
	return 
}


func (v *Version) parse(s string) {
	/*
	 * Version entry order:
	 * DATE TIME LABEL VERSION ORIGFILE FILE AUTHOR ID
	 */

	r := strings.NewReader(s)
	_, err := fmt.Fscanf(r, "%s %s %s %d %s %s %s %s",
		&v.date, &v.time, &v.label, &v.versionNumber, &v.origFile, &v.file, &v.author, &v.id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fscanf: %v\n", err)
	}
}


func isLastVersionChanged(label string) (prevFile string, err error) {
	/*
	 * Check if the file of the previous version is equal to the one archived.
	 * This is done by comparing the sha1 of the file with the id of the archive.
	 * If the file was changed, don't remove it
	 */

	var prevID string
	for _, v := range readVersionsTable() {
		if v.label == label {
			prevID = v.id
			prevFile = v.file
		}
	}

	if prevFile == "none" {
		return
	}

	if prevID != calculateSha1(prevFile) {
		err = fmt.Errorf("WARNING: %s is different from the archived version.", prevFile)
	}
	return
}


func restoreLastVersion(label string) {
	var id string
	var filename string

	f, err := os.Open(VersionsTable)
	if err != nil {
		die(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		entry := new(Version)
		entry.parse(scanner.Text())
		if entry.label == label {
			id = entry.id
			filename = entry.file
		}
	}

	compressed_file := filepath.Join(ArchivesDir, id) + ".gz"
	if err = decompress(compressed_file, filename); err != nil {
		die(err)
	}
	fmt.Printf("Restore previous version: %s\n", filename)
	return
}

func removeLastLine(tableFile string) error {
	var lines []string

	f, err := os.Open(tableFile)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if len(lines) == 0 {
		return nil
	}

	lines = lines[:len(lines)-1]
	output, err := os.Create(tableFile)
	if err != nil {
		return err
	}
	defer output.Close()

	writer := bufio.NewWriter(output)
	for _, line := range lines {
		fmt.Fprintf(writer, "%s\n", line)
	}
	writer.Flush()
	return nil
}

