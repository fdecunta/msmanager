package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

const UserInitials = "FD"

const (
	LocalDir      = "msmanager-data"
	ArchivesDir   = "msmanager-data/archives"
	LabelsTable   = "msmanager-data/labels-table"
	VersionsTable = "msmanager-data/versions-table"
)

func main() {
	log.SetPrefix("msmanager: ")
	log.SetFlags(0)

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

func initDB() {
	dirs := [2]string{LocalDir, ArchivesDir}
	files := [2]string{LabelsTable, VersionsTable}

	for _, d := range dirs {
		err := os.Mkdir(d, 0755)
		if err != nil {
			log.Fatal(err)
		}
	}

	for _, f := range files {
		fptr, err := os.Create(f)
		if err != nil {
			log.Fatal(err)
		}
		fptr.Close()
	}
	fmt.Println("Repository initialized.")
}

func trackLabel(args []string) {
	/*
	 *  To start tracking a label, we need to add the label
	 *  and filename to use to the labels-table, and create an entry
	 *   in the versions-table with the version number 0.
	 */

	if len(args) != 4 {
		fmt.Fprintf(os.Stderr, "Missing arguments.\n")
		usage()
		return
	}

	label := args[2]
	basename := args[3]

	labelsMap := readLabelsMap()
	if _, ok := labelsMap[label]; ok {
		log.Fatal(fmt.Errorf("Label %q already exists.", label))
	}

	writeToLabelsMap(label, basename)
	writeToVersionsTable(Version{
		date:          getDate(),
		time:          getTime(),
		label:         label,
		versionNumber: 0,
		origFile:      "none",
		file:          "none",
		author:        "none",
		id:            "none",
	})
	fmt.Printf("New label %q.\n", label)
}

func updateLabel(args []string) {
	/*
	 * Updates the version of LABEL using the file ORIGFILE
	 *
	 * The function must check that:
	 * - The label exists
	 * - The input file was not used before
	 * - The previous version was not modified. If it was, don't delete it
	 *
	 * If everything is ok, archive the input file, rename the input file using
	 * the filename corresponding to the label, and add an entry to the
	 * versions table.
	 */

	if len(args) != 4 {
		fmt.Println("Missing arguments")
		usage()
	}

	label := args[2]
	origFile := args[3]

	labelsMap := readLabelsMap()
	basename, ok := labelsMap[label]
	if !ok {
		log.Fatal(fmt.Errorf("no such label %q", label))
	}

	id := calculateSha1(origFile)
	newVersionNumber := getLastVersionNumber(label) + 1
	newArchiveFile := filepath.Join(ArchivesDir, id) + ".gz"
	newVersionFile := fmt.Sprintf("%s_%d_%s%s", basename, newVersionNumber, UserInitials, filepath.Ext(origFile))
	email := askAuthorEmail()

	if !askConfirmation(label, origFile, email) {
		fmt.Println("Abort.")
		return
	}

	if _, err := os.Stat(newArchiveFile); err == nil {
		log.Fatal(fmt.Errorf("the same file was used before: \nId: %s", id))
	}

	if err := compress(origFile, newArchiveFile); err != nil {
		log.Fatal(err)
	}

	if err := os.Rename(origFile, newVersionFile); err != nil {
		log.Fatal(err)
	}

	if lastVersionFile, err := isLastVersionChanged(label); err != nil {
		fmt.Println(err, "File not removed.")
	} else {
		if lastVersionFile != "none" {
			os.Remove(lastVersionFile)
		}
	}

	writeToVersionsTable(Version{
		date:          getDate(),
		time:          getTime(),
		label:         label,
		versionNumber: newVersionNumber,
		origFile:      filepath.Base(origFile),
		file:          newVersionFile,
		author:        email,
		id:            id,
	})
	fmt.Printf("Update: %s --> %s\n", origFile, newVersionFile)
}

func printHistory() {
	header := "DATE TIME LABEL VERSION ORIGFILE FILE AUTHOR ID"
	printColumns(header, VersionsTable)
}

func printLabels() {
	header := "LABEL FILENAME"
	printColumns(header, LabelsTable)
}

func restoreFile(args []string) {
	if len(args) < 3 {
		fmt.Println("Missing arguments")
		usage()
	}
	id := args[2]

	var origFile string
	for _, v := range readVersionsTable() {
		if v.id == id {
			origFile = v.origFile
			break
		}
	}
	if len(origFile) == 0 {
		log.Fatal(fmt.Errorf("unable to find ID %s", id))
	}

	compressed_file := filepath.Join(ArchivesDir, id) + ".gz"
	restored_file := fmt.Sprintf("restored_%s", origFile)
	if err := decompress(compressed_file, restored_file); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("File restored: %s\n", restored_file)
}

func undoUpdate() {
	/*
	 * There are two possibilities:
	 * 1. Last command was "track". In that case the
	 *    last entry in versions-table would be a version 0.
	 *    In this case just remove the last entries from
	 *    the labels-table and the versions-table
	 *
	 * 2. Last command was "update".
	 *    In this case we must remove the current
	 *    version and restore the previous from that label.
	 *    To do this:
	 *	- Remove the archived file. Rename the current
	 *	  version to it's original name.
	 *	- Restore the previous version.
	 *    Then delete the last entry from versions-table.
	 */

	versionsTable := readVersionsTable()
	lastEntry := versionsTable[len(versionsTable)-1]

	if lastEntry.versionNumber == 0 {
		if err := removeLastLine(LabelsTable); err != nil {
			log.Fatal(err)
		}
		if err := removeLastLine(VersionsTable); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Remove label %q.\n", lastEntry.label)
	} else {
		compressed_file := filepath.Join(ArchivesDir, lastEntry.id) + ".gz"
		os.Remove(compressed_file)
		os.Rename(lastEntry.file, lastEntry.origFile)
		fmt.Printf("Rename: %s ---> %s\n", lastEntry.file, lastEntry.origFile)

		if err := removeLastLine(VersionsTable); err != nil {
			log.Fatal(err)
		}
		if lastEntry.versionNumber > 1 {
			restoreLastVersion(lastEntry.label)
		}
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
