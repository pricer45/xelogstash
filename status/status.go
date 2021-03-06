package status

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/billgraziano/xelogstash/log"
	"github.com/pkg/errors"
)

// key is used to define how a file is generated and preven duplicates
// type key struct {
// 	Prefix     string
// 	Type       string
// 	Instance   string
// 	Identifier string
// }

var sources map[string]bool
var mux sync.Mutex

//ErrDup indicates a duplicate was found
var ErrDup = errors.New("duplicate domain-instance-class-id")

func init() {
	sources = make(map[string]bool)
	//mux = sync.Mutex
}

// File is a way to keep track of state
type File struct {
	// prefix         string
	// instance       string
	// session        string
	Name string
	file *os.File
}

const (
	// StateSuccess means that we will read the file normally
	StateSuccess = "good"

	// StateReset means that we will assume a bad file name and offset
	StateReset = "reset"
)

const (
	// ClassXE is used for XE sessions
	ClassXE = "XE"
	// ClassAgentJobs is used for AGENT job history
	ClassAgentJobs = "JOBS"
)

// CheckDupe checks to see if this session has been processed already
// If so, it returns an error
// It saves all sessions
// class is XE, AUDIT, or AGENT
// session is the XE session name or Audit session name
func CheckDupe(domain, instance, class, id string) error {

	fileName := strings.ToLower(fileName(domain, instance, class, id))

	mux.Lock()
	defer mux.Unlock()

	_, found := sources[fileName]
	if found {
		return ErrDup
	}

	sources[fileName] = true

	return nil
}

// NewFile generates a new state file for this domain, instance, session
// This also creates the state file if it doesn't exist
func NewFile(domain, instance, class, id string) (File, error) {
	var f File
	var err error

	// Get EXE directory
	executable, err := os.Executable()
	if err != nil {
		return f, errors.Wrap(err, "os.executable")
	}
	exeDir := filepath.Dir(executable)

	stateDir := filepath.Join(exeDir, "xestate")
	if _, err = os.Stat(stateDir); os.IsNotExist(err) {
		err = os.Mkdir(stateDir, 0644)
	}
	if err != nil {
		return f, errors.Wrap(err, "os.mkdir")
	}

	f.Name = filepath.Join(stateDir, fileName(domain, instance, class, id))
	return f, nil
}

// GetOffset returns the last file and offset for this file state
func (f *File) GetOffset() (fileName string, offset int64, xestatus string, err error) {

	var fp *os.File
	_, err = os.Stat(f.Name)
	if os.IsNotExist(err) {
		fp, err = os.OpenFile(f.Name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return "", 0, StateReset, errors.Wrap(err, "create")
		}
		f.file = fp
		return "", 0, StateSuccess, nil
	} else if err != nil {
		return "", 0, StateReset, errors.Wrap(err, "stat")
	}

	readonly, err := os.OpenFile(f.Name, os.O_RDONLY, 0666)
	if err != nil {
		return "", 0, StateReset, errors.Wrap(err, "openreadonly")
	}

	var line []string
	reader := csv.NewReader(bufio.NewReader(readonly))
	for {
		line, err = reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return "", 0, StateReset, errors.Wrap(err, "read")
		}
		if len(line) < 2 || len(line) > 3 {
			return "", 0, StateReset, errors.Errorf("len(line) expected: 2 or 3; got %d (%v)", len(line), line)
		}

		fileName = strings.TrimSpace(line[0])
		offset, err = strconv.ParseInt(strings.TrimSpace(line[1]), 10, 64)
		if err != nil {
			return "", 0, StateReset, errors.Errorf("error reading offset: got %s", line[1])
		}
		if len(line) == 2 {
			xestatus = StateSuccess // Assume we are good
		} else {
			xestatus = strings.TrimSpace(line[2])
		}
	}
	err = readonly.Close()
	if err != nil {
		return "", 0, StateReset, errors.Wrap(err, "close")
	}

	// TODO close & reopen the file
	fp, err = os.OpenFile(f.Name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return "", 0, StateReset, errors.Wrap(err, "openappend")
	}

	_, err = fp.Stat()
	if err != nil {
		return fileName, offset, StateReset, errors.Wrap(err, "stat-2")
	}

	f.file = fp

	return fileName, offset, xestatus, nil
}

// FileName returns the base file name to track state
func fileName(domain, instance, class, id string) string {
	var fileName string
	instance = strings.Replace(instance, "\\", "__", -1)

	// use it to build the file name
	// if prefix == "" {
	// 	fileName = fmt.Sprintf("%s_%s_%s.state", instance, class, id)
	// } else {
	fileName = fmt.Sprintf("%s_%s_%s_%s.state", domain, instance, class, id)
	//}
	return fileName
}

// FileName returns the base file name to track status
func legacyFileName(prefix, instance, class, id string) string {
	var fileName string
	instance = strings.Replace(instance, "\\", "__", -1)

	// use it to build the file name
	if prefix == "" {
		fileName = fmt.Sprintf("%s_%s_%s.status", instance, class, id)
	} else {
		fileName = fmt.Sprintf("%s_%s_%s_%s.status", prefix, instance, class, id)
	}
	return fileName
}

// Save persists the last filename and offset that was successfully completed
func (f *File) Save(fileName string, offset int64, xestatus string) error {
	if f.file == nil {
		return errors.New("state file not open")
	}

	err := writeState(f.file, fileName, offset, xestatus)
	if err != nil {
		return errors.Wrap(err, "writeStatus")
	}

	return nil
}

func writeState(f *os.File, xeFileName string, offset int64, xestatus string) error {
	msg := fmt.Sprintf("%s, %d, %s\r\n", xeFileName, offset, xestatus)
	_, err := f.WriteString(msg)
	if err != nil {
		return errors.Wrap(err, "file.write")
	}
	return nil
}

// Done closes the file
func (f *File) Done(xeFileName string, offset int64, xestatus string) error {
	var err error
	err = f.Save(xeFileName, offset, xestatus)
	if err != nil {
		return errors.Wrap(err, "save")
	}

	err = f.file.Close()
	if err != nil {
		return errors.Wrap(err, "close")
	}

	if f.Name == "" {
		return errors.New("f.Name is empty")
	}

	// Delete the .0 file
	safetyFileName := fmt.Sprintf("%s.0", f.Name)
	_, err = os.Stat(safetyFileName)
	if err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err, "stat")
		}
	}

	if os.IsExist(err) {
		err = os.Remove(safetyFileName)
		if err != nil {
			return errors.Wrap(err, "remove")
		}
	}

	// Rename to the .0 file
	err = os.Rename(f.Name, safetyFileName)
	if err != nil {
		return errors.Wrap(err, "rename")
	}

	// Write the new file
	newStatusFile, err := os.OpenFile(f.Name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return errors.Wrap(err, "create")
	}

	err = writeState(newStatusFile, xeFileName, offset, xestatus)
	if err != nil {
		return errors.Wrap(err, "writestate")
	}

	err = newStatusFile.Close()
	if err != nil {
		return errors.Wrap(err, "close")
	}

	return nil
}

// SwitchV2 moves to new dir and name scheme
func SwitchV2(wid int, prefix, domain, instance, class, session string) error {
	mux.Lock()
	defer mux.Unlock()
	var msg string
	/*
		1. Get old file name
		2. Get new file name
		3. move the file
	*/

	// Get EXE directory
	executable, err := os.Executable()
	if err != nil {
		return errors.Wrap(err, "os.executable")
	}
	exeDir := filepath.Dir(executable)

	legacyDir := filepath.Join(exeDir, "status")
	legacyFile := filepath.Join(legacyDir, legacyFileName(prefix, instance, class, session))

	// if old dir (/status) doesn't exist, we're done
	_, err = os.Stat(legacyDir)
	if os.IsNotExist(err) {
		return nil
	}

	// does the old file exist?
	_, err = os.Stat(legacyFile)
	if os.IsNotExist(err) {
		return nil
	}

	log.Debug(fmt.Sprintf("[%d] Legacy status file: %s", wid, legacyFile))
	newDir := filepath.Join(exeDir, "xestate")
	newFile := filepath.Join(newDir, fileName(domain, instance, class, session))

	// make the new state directory if it doesn't exist
	if _, err = os.Stat(newDir); os.IsNotExist(err) {
		msg = fmt.Sprintf("[%d] Making new state directory: %s", wid, newDir)
		log.Info(msg)
		err = os.Mkdir(newDir, 0666)
	}
	if err != nil {
		return errors.Wrap(err, "os.mkdir")
	}

	// does the new file exist?
	_, err = os.Stat(newFile)
	if !os.IsNotExist(err) {
		return fmt.Errorf("NEW STATE FILE ALREADY EXISTS: %s", newFile)
	}

	// Move the file
	msg = fmt.Sprintf("[%d] Moving %s\\%s to %s\\%s", wid, "status", filepath.Base(legacyFile), "xestate", filepath.Base(newFile))
	log.Info(msg)
	err = os.Rename(legacyFile, newFile)
	if err != nil {
		return errors.Wrap(err, "os.rename")
	}

	// Remove the .0 file
	zeroFile := legacyFile + ".0"
	_, err = os.Stat(zeroFile)
	if !os.IsNotExist(err) {
		msg = fmt.Sprintf("[%d] Removing temp file %s\\%s", wid, "status", filepath.Base(zeroFile))
		log.Info(msg)
		err = os.Remove(zeroFile)
		if err != nil {
			return errors.Wrap(err, "os.remove")
		}
	}
	return nil
}
