package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const UserInitials = "FD"

const (
	LocalDir      = "msmanager-data"
	ArchivesDir   = "msmanager-data/archives"
	LabelsTable   = "msmanager-data/labels-table"
	VersionsTable = "msmanager-data/versions-table"
)


type Version struct {
	date           string
	time           string
	label          string
	versionNumber  int
	origFile       string
	file           string
	author         string
	id             string
}


func main() {
	if len(os.Args) == 1 {
		usage()
		return
	}

	if _, err := os.Stat(LocalDir); err != nil && os.Args[1] != "init" {
		fmt.Printf("No repository in current directory. Use %q\n\n", "init")
		usage()
		return
	}

	switch os.Args[1] {
	case "init":
		initDB()
	case "track":
		trackLabel(os.Args)
	case "update":
		updateLabel(os.Args)
	case "hist":
		printHistory()
	case "labels":
		printLabels()
	case "restore":
		restoreFile(os.Args)
	case "undo":
		undoUpdate()
	default:
		usage()
	}
}


func usage() {
	fmt.Println("usage: msmanager")
	fmt.Println("Commands:")
	fmt.Println("  init                        Initialize a new repository")
	fmt.Println("  track <label> <basename>    Start tracking label, naming files with <basename>")
	fmt.Println("  update <label> <file>       Update version of label with file")
	fmt.Println("  hist                        Show versions history")
	fmt.Println("  labels                      Print labels and their basenames")
	fmt.Println("  restore <ID>                Restore a file")
	fmt.Println("  undo                        Undo the last command")
	os.Exit(0)
}


func initDB() {
	dirs  := [2]string{LocalDir, ArchivesDir}
	files := [2]string{LabelsTable, VersionsTable}

	for _, d := range dirs {
		err := os.Mkdir(d, 0755)
		if err != nil {
			die(err)
		}
	}

	for _, f := range files {
		fptr, err := os.Create(f)
		if err != nil {
			die(err)
		}
		fptr.Close()
	}
	fmt.Println("Repository initialized.")
}


func trackLabel(args []string) {

	/*
	 *  To start tracking a label, we need to add the label
	 *  and basename to the labels-table, and create an entry
	 *   in the versions-table with the version number 0.
	 */

	if len(args) != 4 {
		fmt.Fprintf(os.Stderr, "Missing arguments.\n")
		usage()
	}
	label := args[2]
	basename := args[3]

	Labels := readLabelsTable()
	if _, ok := Labels[label]; ok {
		die(fmt.Errorf("Label %q already exists.", label))
	}

	f, err := os.OpenFile(LabelsTable, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		die(err)
	}

	/* Labels-table has two columns: LABEL BASENAME */

	fmt.Fprintf(f, "%s %s\n", label, basename)
	f.Close()

	writeToVersionsTable(Version{
		date:            getDate(),
		time:            getTime(),
		label:           label,
		versionNumber:   0,
		origFile:        "none",
		file:            "none",
		author:          "none",
		id:              "none",
		})

	fmt.Println("Label added.")
}


func updateLabel(args []string) {

	/*
	 * Updates the version of LABEL using the file ORIGFILE
	 * 
	 * - Calculate the sha1 of ORIGFILE and uses it as an ID.
	 *   Check that this ID was not used (i.e., check that 
	 *   the file was not used)
	 * - Compress the file into the ArchivesDir. The compressed
	 *   file is named {ID}.gz
	 * - Adds a new entry to the VersionsTable
	 */	

	if len(args) != 4 {
		fmt.Println("Missing arguments")
		usage()
	}

	label := args[2]
	origFile := args[3]

	Basenames := readLabelsTable()
	// Versions := readVerionsTable()

	basename, ok := Basenames[label]
	if !ok {
		die(fmt.Errorf("no such label %q", label))
	}

	id, err := calculateSha1(origFile)
	if err != nil {
		die(err)
	}
	
	if _, err := os.Stat(filepath.Join(ArchivesDir, id) + ".gz"); err == nil {
		die(fmt.Errorf("file already used. \nId: %s", id))
	}

	email := askAuthorEmail()
	if !askConfirmation(label, origFile, email) {
		fmt.Println("Abort.")
		return
	}

	if err = compress(origFile, filepath.Join(ArchivesDir, id) + ".gz"); err != nil {
		die(err)
	}

	versionNumber := 1 + getLastVersionNumber(label)

	newFile := fmt.Sprintf("%s_%d_%s%s", basename, versionNumber, UserInitials, filepath.Ext(origFile))
	if err = os.Rename(origFile, newFile); err != nil {		
		die(err)
	} 

	handlePreviousVersion(label)

	writeToVersionsTable(Version{
		date:           getDate(),
		time:           getTime(),
		label:          label,
		versionNumber:  versionNumber,
		origFile:       filepath.Base(origFile),
		file:           newFile,
		author:         email,
		id:             id,	
		})

	fmt.Printf("Update: %s --> %s\n", origFile, newFile)
}

func handlePreviousVersion(label string) {
	var prevID string
	var prevFile string

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
			prevID = entry.id
			prevFile = entry.file
		}
	}

	if prevFile == "none" {
		return
	}

	sha1, err := calculateSha1(prevFile)
	if err != nil {
		die(err)
	}

	if sha1 != prevID {
		fmt.Println("WARNING: the previous version seems to be different from the file archived")
		fmt.Printf("sha1 from %s is different to %s\n", prevFile, prevID)
		fmt.Println("The file will not be removed")
	} else { 
		os.Remove(prevFile)
		fmt.Println("Previous version archived.")
	}
}


func printHistory() {
	header := "DATE TIME LABEL VERSION ORIGFILE FILE AUTHOR ID"
	printColumns(header, VersionsTable)
}

func printLabels() {
	header := "LABEL BASENAME"
	printColumns(header, LabelsTable)
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


func askAuthorEmail() string {
	fmt.Printf("Author email: ")
	var email string
	_, err := fmt.Scan(&email)
	if err != nil {
		die(err)
	}
	return email
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

func getLastVersionNumber(label string) int {
	LastVersion := 0

	f, err := os.Open(VersionsTable)
	if err != nil {
		die(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		entry := new(Version)
		entry.parse(scanner.Text())
		if entry.label == label && entry.versionNumber > LastVersion {
			LastVersion = entry.versionNumber
		}
	}

	if err = scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading versions-table in getLastVersionNumber():", err)
		die(err)
	}

	return LastVersion
}


func restoreFile(args []string) {
	if len(args) < 3 {
		fmt.Println("Missing arguments")
		usage()
	}
	id := args[2]
	compressed_file := filepath.Join(ArchivesDir, id) + ".gz"
	restored_file := fmt.Sprintf("restored_%s", getOrigFilename(id))
	if err := decompress(compressed_file, restored_file); err != nil {
		die(err)
	}
	fmt.Printf("File restored: %s\n", restored_file)
}


func getOrigFilename(id string) string {
	f, err := os.Open(VersionsTable)
	if err != nil {
		die(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		v := new(Version)
		v.parse(scanner.Text())
		if v.id == id {
			return v.origFile
		}
	}
	fmt.Printf("Error: Can't find basename in versions history for %s\n", id)
	os.Exit(1)
	return ""
}


func undoUpdate() {
	lastEntry := new(Version)	

	f, err := os.Open(VersionsTable)
	if err != nil {
		die(err)
	}
	defer f.Close()
	
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lastEntry.parse(scanner.Text())
	}

	/*
	 * There are possibilities:
	 * 1. Last action was the creation of a Label with 'track'. 
	 *    In this case just remove the last entry from 
	 *    the labels-table and the versions-table
	 * 
	 * 2. Last action was an update.	
	 *    In this case the program must remove the current
	 *    version and restore the previous from that label.
	 *    To do this: 
	 *	- Remove the archived file. Rename the current
	 *	  version to it's original name.
	 *	- Restore the previous version.
	 *    Then delete the last entry from versions-table.
	 */

	if lastEntry.versionNumber == 0 {
		if err := removeLastLine(LabelsTable); err != nil {
			die(err)
		}
		if err := removeLastLine(VersionsTable); err != nil {
			die(err)
		}	
	} else {
		compressed_file := filepath.Join(ArchivesDir, lastEntry.id) + ".gz"
		os.Remove(compressed_file)
		os.Rename(lastEntry.file, lastEntry.origFile)
		fmt.Printf("Rename: %s ---> %s\n", lastEntry.file, lastEntry.origFile)

		if err := removeLastLine(VersionsTable); err != nil {
			die(err)
		}
		if lastEntry.versionNumber > 1 {
			restoreLastVersion(lastEntry.label)
		}
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

	lines = lines[ :len(lines)-1]
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


func readVersionsTable() []*Version {
	var allVersions []*Version

	f, err := os.Open(VersionsTable)
	if err != nil {
		die(err)
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		v := new(Version)
		v.parse(scanner.Text())
		allVersions = append(allVersions, v)
	}
	if err := scanner.Err(); err != nil {
		die(err)
	}

	return allVersions
}
