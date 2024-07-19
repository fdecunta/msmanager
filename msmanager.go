package main

import (
	"bufio"
	"compress/gzip"
	"crypto/sha1"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)


const UserInitials = "FD"

const (
	LocalDir      = "msmanager-data"
	ArchivesDir   = "msmanager-data/archives"
	LogsDir       = "msmanager-data/logs"
	LabelsTable   = "msmanager-data/labels-table"
	VersionsTable = "msmanager-data/versions-table"
)

type LabelEntry struct {
	label    string
	basename string
}

type VersionsEntry struct {
	date    string
	time    string
	label   string
	version int
	file    string
	id      string
	author  string
}

func main() {
	if len(os.Args) == 1 {
		usage()
		return
	}

	if !fileExists(LocalDir) && os.Args[1] != "init" {
		fmt.Println("No repository in current directory")
		usage()
		return
	}

	switch os.Args[1] {
	case "init":
		initDB()
	case "track":
		handleTrack(os.Args)
	case "update":
		handleUpdate(os.Args)
	case "hist":
		printHistory()
	case "labels":
		printLabels()
	case "log":
		printLogs()
	case "restore":
		restoreFile(os.Args)
	case "undo":
		fmt.Println("undo")
	default:
		usage()
		return
	}
}

func usage() {
	fmt.Println("usage: msmanager")
	fmt.Println("Commands:")
	fmt.Println("  init                        Initialize a new repository")
	fmt.Println("  track <label> <basename>    Start tracking label")
	fmt.Println("  update <label> <file>       Updates a label with file")
	fmt.Println("  hist                        Show history")
	fmt.Println("  labels                      Print tags and their base filenames")
	fmt.Println("  log                         View commit history")
	fmt.Println("  restore <ID>                Restore a file")
	fmt.Println("  undo                        Undo the last update")
	os.Exit(1)
}

func initDB() {
	dirs := [3]string{LocalDir, ArchivesDir, LogsDir}
	files := [2]string{LabelsTable, VersionsTable}

	for _, d := range dirs {
		errd := os.Mkdir(d, 0755)
		if errd != nil {
			log.Fatal(errd)
		}
	}

	for _, f := range files {
		fptr, errf := os.Create(f)
		if errf != nil {
			log.Fatal(errf)
		}
		fptr.Close()
	}
	fmt.Println("Repository initialized.")
}

func handleTrack(args []string) {
	if len(os.Args) != 4 {
		fmt.Println("Missing arguments")
		usage()
	}

	label := args[2]
	basename := args[3]

	if labelExists(label) {
		fmt.Println("Label already exists.")
		return
	}
	trackLabel(label, basename)	
}

func trackLabel(label string, basename string) {
	/*
	 *  Starts tracking a label with a given basename.
	 *
	 *  Adds the label and basename to the labels-table
	 *  and creates the first entry in the history-table
	 */

	newLabel := LabelEntry{label, basename}
	newLabel.writeToLabelsTable()

	newVersion := VersionsEntry{
		getDate(),
		getTime(),
		label,
		0,
		"none",
		"0",
		"none",
	}
	newVersion.writeToVersionsTable()
	fmt.Println("Label added.")
}

func labelExists(label string) bool {
	rval := false

	f, err := os.Open(LabelsTable)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		s := strings.Split(line, " ")
		if label == s[0] {
			rval = true
			break
		}
	}

	if err = scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading history-file:", err)
	}
	return rval
}

func fileExists(filePath string) bool {
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
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

func printHistory() {
	header := "DATE TIME LABEL VERSION FILE ID AUTHOR"
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
		log.Fatal(err)
	}
	defer stdin.Close()

	f, ferr := os.Open(file)
	if ferr != nil {
		log.Fatal(ferr)
	}
	defer f.Close()

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
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
		log.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		log.Fatal(err)
	}
}


func handleUpdate(args []string) {
	if len(args) != 4 {
		fmt.Println("Missing arguments")
		usage()
	}

	label := args[2]
	file := args[3]

	if !labelExists(label) {
		fmt.Fprintln(os.Stderr, "Error: Label did not exist.")
		return
	}
	if !fileExists(file) {
		fmt.Fprintln(os.Stderr, "Error: unable to find file.")
		return
	}

	update(label, file)
}

func update(label string, file string) {
	/*
	 * Updates the version if LABEL using the file FILE
	 * 
	 * - Calculates the sha1 of FILE and uses it as an ID.
	 *   Check that this ID was not used (i.e., check if 
	 *   the file was not used)
	 * - Compress the file into the ArchivesDir. The compressed
	 *   file is named {ID}.gz
	 * - Adds a new entry to the VersionsTable
	 * - Adds a new log entry
	*/	

	id, err := calculateSha1(file)
	if err != nil {
		log.Fatal(err)
	}
	if isArchived(id) {
		fmt.Fprintf(os.Stderr, "Error: file was already used.\nID: %s\n", id)
		return
	}

	email := getAuthorEmail()
	if !confirmUpdate(label, file, email) {
		fmt.Println("Abort.")
		return
	}

	if err = gzipFile(file, ArchivesDir, id); err != nil {
		fmt.Println("Error: can't compress file")
		return
	}

	versionNumber := 1 + getLastVersionNumber(label)
	basename := getBasename(label)
	newFilename := fmt.Sprintf("%s_%d_%s.docx", basename, versionNumber, UserInitials)
	if err = os.Rename(file, newFilename); err != nil {
		log.Fatal(err)
	}

	//
	// TODO: Check if the previous version was archived
	//

	newVersion := VersionsEntry{
		getDate(),
		getTime(),
		label,
		versionNumber,
		newFilename,
		id,
		email,
	}

	newVersion.writeToVersionsTable()
	newVersion.writeLog(filepath.Base(file))
}

func getBasename(label string) string {
	f, err := os.Open(LabelsTable)
	if err != nil {
		log.Fatal(err)		
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.Split(scanner.Text(), " ")
		if line[0] == label {
			return line[1]
		}
	}
	return ""
}

func calculateSha1(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func isArchived(id string) bool {
	archive_file := id + ".gz"
	if fileExists(archive_file) {
		return true
	} else {
		return false
	}
}

func getAuthorEmail() string {
	fmt.Printf("Author email: ")
	var email string
	_, err := fmt.Scan(&email)
	if err != nil {
		log.Fatal(err)
	}
	return email
}

func confirmUpdate(label string, file string, email string) bool {
	fmt.Println()
	fmt.Printf("Label: %s\n", label)
	fmt.Printf("File : %s\n", file)
	fmt.Printf("Email: %s\n", email)
	fmt.Printf("Confirm update? (y/n): ")

	var ans string
	_, err := fmt.Scan(&ans)
	if err != nil {
		log.Fatal(err)
	}

	if ans == "y" || ans == "yes" {
		return true
	} else {
		return false
	}
}

func gzipFile(inputFile, outputDir, id string) error {
	// Open the input file
	inFile, err := os.Open(inputFile)
	if err != nil {
		return err
	}
	defer inFile.Close()

	// Create the output file
	outputFile := filepath.Join(outputDir, id+".gz")
	outFile, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer outFile.Close()

	// Create a gzip writer on top of the output file
	gzipWriter := gzip.NewWriter(outFile)
	defer gzipWriter.Close()

	// Copy the input file to the gzip writer
	if _, err := io.Copy(gzipWriter, inFile); err != nil {
		return err
	}

	return nil
}

func gunzipFile(inputFile string, outputFile string) error {
	// Open the input file
	inFile, err := os.Open(inputFile)
	if err != nil {
		return err
	}
	defer inFile.Close()

	// Create a gzip reader on top of the input file
	gzipReader, err := gzip.NewReader(inFile)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	// Create the output file
	outFile, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer outFile.Close()

	// Copy the content from the gzip reader to the output file
	if _, err := io.Copy(outFile, gzipReader); err != nil {
		return err
	}

	return nil
}

func (l LabelEntry) writeToLabelsTable() {
	f, err := os.OpenFile(LabelsTable, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	/*
	 * The labels-table has two columns:
	 *    LABEL BASENAME
	 */
	fmt.Fprintf(f, "%s %s\n", l.label, l.basename)
	f.Close()
}

func (v VersionsEntry) writeToVersionsTable() {
	f, err := os.OpenFile(VersionsTable, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	/*
	 *  The history-table has this columns:
	 *  DATE TIME LABEL VERSION FILE ID AUTHOR
	 */
	fmt.Fprintf(f, "%s %s %s %d %s %s %s\n", v.date, v.time, v.label, v.version, v.file, v.id, v.author)
	f.Close()
}

func (v *VersionsEntry) parse(s string) {
	r := strings.NewReader(s)
	_, err := fmt.Fscanf(r, "%s %s %s %d %s %s %s",
		&v.date, &v.time, &v.label, &v.version, &v.file, &v.id, &v.author)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fscanf: %v\n", err)
	}
}

func getLastVersionNumber(label string) int {
	LastVersion := 0

	f, err := os.Open(VersionsTable)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		vEntry := new(VersionsEntry)
		vEntry.parse(scanner.Text())
		if vEntry.label == label && vEntry.version > LastVersion {
			LastVersion = vEntry.version
		}
	}

	if err = scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading versions-table in getLastVersionNumber():", err)
		log.Fatal(err)
	}

	return LastVersion
}

func (v VersionsEntry) writeLog(oldFilename string) {
	f, err := os.Create(filepath.Join(LogsDir, v.id))
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	fmt.Fprintf(w, "ID       : %s\n", v.id)
	fmt.Fprintf(w, "Label    : %s\n", v.label)
	fmt.Fprintf(w, "Version  : %d\n", v.version)
	fmt.Fprintf(w, "Date     : %s\n", v.date)
	fmt.Fprintf(w, "Time     : %s\n", v.time)
	fmt.Fprintf(w, "Author   : %s\n", v.author)
	fmt.Fprintf(w, "OrigFile : %s\n", oldFilename)
	fmt.Fprintf(w, "File     : %s\n", v.file)
	w.Flush()
}


func printLogs() {
	for _, id := range getAllIds() {
		f, err := os.Open(filepath.Join(LogsDir, id))
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
	
		_, err = io.Copy(os.Stdout, f)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println()
	}
}


func getAllIds() []string {
	ids := make([]string, 0)

	f, err := os.Open(VersionsTable)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		entry := new(VersionsEntry)
		entry.parse(scanner.Text())
		if entry.version > 0 {
			ids = append(ids, entry.id)
		}
	}
	return ids
}


func restoreFile(args []string) {
	if len(args) < 3 {
		fmt.Println("Missing arguments")
		usage()
	}

	id := args[2]
	compressed_file := filepath.Join(ArchivesDir, id) + ".gz"
	
	if !fileExists(compressed_file) {
		fmt.Printf("Unable to find file: %s\n", compressed_file)
		return
	}

	restored_file := fmt.Sprintf("restored_%s", getOrigFilename(id))
	if err := gunzipFile(compressed_file, restored_file); err != nil {
		fmt.Println("Error decompressing file")
		os.Exit(1)
	}
	fmt.Printf("File restored: %s\n", restored_file)
}


func getOrigFilename(id string) string {
	f, err := os.Open(filepath.Join(LogsDir, id))
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "OrigFile") {
			s := strings.Split(scanner.Text(), ":")
			return strings.Trim(s[1], " ")
		}
	}
	return ""
}
