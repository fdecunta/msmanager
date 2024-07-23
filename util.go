package main

import (
	"bufio"
	"compress/gzip"
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)


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


func die(err error) {
	fmt.Fprintf(os.Stderr, "Error: %s\n", err)
	os.Exit(1)
}


func readLabelsTable() map[string]string {
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
