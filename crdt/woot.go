package crdt

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

type Document struct {
	Characters []Character
}

type Character struct {
	ID         string
	Visible    bool
	Value      string
	IDPrevious string
	IDNext     string
}

var (
	mu         sync.Mutex
	SiteID     = 0
	LocalClock = 0

	CharacterStart = Character{
		ID:         "start",
		Visible:    false,
		Value:      "",
		IDPrevious: "",
		IDNext:     "end"}

	CharacterEnd = Character{
		ID:         "end",
		Visible:    false,
		Value:      "",
		IDPrevious: "start",
		IDNext:     ""}

	ErrPositionOutOfBounds = errors.New("position out of bounds")
	ErrEmptyWCharacter     = errors.New("empty char ID provided")
	ErrBoundsNotPresent    = errors.New("subsequence bound(s) not present")
)

func New() Document {
	return Document{Characters: []Character{CharacterStart, CharacterEnd}}
}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func Load(fileName string) (Document, error) {
	doc := New()
	content, err := os.ReadFile(fileName)
	if err != nil {
		return doc, err
	}
	lines := strings.Split(string(content), "\n")
	pos := 1
	for i := 0; i < len(lines); i++ {
		for j := 0; j < len(lines[i]); j++ {
			_, err := doc.Insert(pos, string(lines[i][j]))
			if err != nil {
				return doc, err
			}
			pos++
		}
		if i < len(lines)-1 {
			_, err := doc.Insert(pos, "\n")
			if err != nil {
				return doc, err
			}
			pos++
		}
	}
	return doc, err
}

func Save(fileName string, doc *Document) error {
	return os.WriteFile(fileName, []byte(Content(*doc)), 0644)
}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func (doc *Document) SetText(newDoc Document) {
	for _, char := range newDoc.Characters {
		c := Character{ID: char.ID, Visible: char.Visible, Value: char.Value, IDPrevious: char.IDPrevious, IDNext: char.IDNext}
		doc.Characters = append(doc.Characters, c)
	}
}

func Content(doc Document) string {
	value := ""
	for _, char := range doc.Characters {
		if char.Visible {
			value += char.Value
		}
	}
	return value
}

func IthVisible(doc Document, position int) Character {
	count := 0

	for _, char := range doc.Characters {
		if char.Visible {
			if count == position-1 {
				return char
			}
			count++
		}
	}

	return Character{ID: "-1"}
}

func (doc *Document) Length() int {
	return len(doc.Characters)
}

func (doc *Document) ElementAt(position int) (Character, error) {
	if position < 0 || position >= doc.Length() {
		return Character{}, ErrPositionOutOfBounds
	}

	return doc.Characters[position], nil
}

func (doc *Document) Position(charID string) int {
	for position, char := range doc.Characters {
		if charID == char.ID {
			return position + 1
		}
	}

	return -1
}

func (doc *Document) Left(charID string) string {
	i := doc.Position(charID)
	if i <= 0 {
		return doc.Characters[i].ID
	}
	return doc.Characters[i-1].ID
}

func (doc *Document) Right(charID string) string {
	i := doc.Position(charID)
	if i >= len(doc.Characters)-1 {
		return doc.Characters[i-1].ID
	}
	return doc.Characters[i+1].ID
}

func (doc *Document) Contains(charID string) bool {
	position := doc.Position(charID)
	return position != -1
}

func (doc *Document) Find(id string) Character {
	for _, char := range doc.Characters {
		if char.ID == id {
			return char
		}
	}

	return Character{ID: "-1"}
}

func (doc *Document) Subseq(wcharacterStart, wcharacterEnd Character) ([]Character, error) {
	startPosition := doc.Position(wcharacterStart.ID)
	endPosition := doc.Position(wcharacterEnd.ID)

	if startPosition == -1 || endPosition == -1 {
		return doc.Characters, ErrBoundsNotPresent
	}

	if startPosition > endPosition {
		return doc.Characters, ErrBoundsNotPresent
	}

	if startPosition == endPosition {
		return []Character{}, nil
	}

	return doc.Characters[startPosition : endPosition-1], nil
}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func (doc *Document) LocalInsert(char Character, position int) (*Document, error) {
	if position <= 0 || position >= doc.Length() {
		return doc, ErrPositionOutOfBounds
	}

	if char.ID == "" {
		return doc, ErrEmptyWCharacter
	}

	doc.Characters = append(doc.Characters[:position],
		append([]Character{char}, doc.Characters[position:]...)...,
	)

	doc.Characters[position-1].IDNext = char.ID
	doc.Characters[position+1].IDPrevious = char.ID

	return doc, nil
}

func (doc *Document) IntegrateInsert(char, charPrev, charNext Character) (*Document, error) {
	subsequence, err := doc.Subseq(charPrev, charNext)
	if err != nil {
		return doc, err
	}

	position := doc.Position(charNext.ID)
	position--

	if len(subsequence) == 0 {
		return doc.LocalInsert(char, position)
	}

	if len(subsequence) == 1 {
		return doc.LocalInsert(char, position-1)
	}

	i := 1
	for i < len(subsequence)-1 && subsequence[i].ID < char.ID {
		i++
	}
	return doc.IntegrateInsert(char, subsequence[i-1], subsequence[i])
}

func (doc *Document) GenerateInsert(position int, value string) (*Document, error) {
	mu.Lock()
	LocalClock++
	mu.Unlock()

	charPrev := IthVisible(*doc, position-1)
	charNext := IthVisible(*doc, position)

	if charPrev.ID == "-1" {
		charPrev = doc.Find("start")
	}
	if charNext.ID == "-1" {
		charNext = doc.Find("end")
	}

	char := Character{
		ID:         fmt.Sprint(SiteID) + fmt.Sprint(LocalClock),
		Visible:    true,
		Value:      value,
		IDPrevious: charPrev.ID,
		IDNext:     charNext.ID,
	}

	return doc.IntegrateInsert(char, charPrev, charNext)
}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func (doc *Document) IntegrateDelete(char Character) *Document {
	position := doc.Position(char.ID)
	if position == -1 {
		return doc
	}

	doc.Characters[position-1].Visible = false

	return doc
}

func (doc *Document) GenerateDelete(position int) *Document {
	char := IthVisible(*doc, position)
	return doc.IntegrateDelete(char)
}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func (doc *Document) Insert(position int, value string) (string, error) {
	newDoc, err := doc.GenerateInsert(position, value)
	if err != nil {
		return Content(*doc), err
	}

	return Content(*newDoc), nil
}

func (doc *Document) Delete(position int) string {
	newDoc := doc.GenerateDelete(position)
	return Content(*newDoc)
}
