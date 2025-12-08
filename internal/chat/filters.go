package chat

import (
	"context"
	"regexp"
	"strings"
	"sync"
)

// FilterMode defines how the filter handles matched content
type FilterMode int

const (
	// FilterModeBlock blocks the entire message
	FilterModeBlock FilterMode = iota
	// FilterModeMask replaces matched words with asterisks
	FilterModeMask
	// FilterModeReplace replaces matched words with a custom string
	FilterModeReplace
	// FilterModeLog logs but allows the message through
	FilterModeLog
)

// ProfanityFilter implements ContentFilter for profanity/inappropriate content
type ProfanityFilter struct {
	words         map[string]bool      // Lowercase blocked words
	patterns      []*regexp.Regexp     // Regex patterns to match
	mode          FilterMode           // How to handle matches
	replacement   string               // Replacement string for FilterModeReplace
	mu            sync.RWMutex
}

// NewProfanityFilter creates a new profanity filter
func NewProfanityFilter(mode FilterMode) *ProfanityFilter {
	return &ProfanityFilter{
		words:       make(map[string]bool),
		patterns:    make([]*regexp.Regexp, 0),
		mode:        mode,
		replacement: "[filtered]",
	}
}

// Name returns the filter name
func (f *ProfanityFilter) Name() string {
	return "profanity"
}

// AddWords adds words to the block list
func (f *ProfanityFilter) AddWords(words ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, word := range words {
		f.words[strings.ToLower(word)] = true
	}
}

// RemoveWords removes words from the block list
func (f *ProfanityFilter) RemoveWords(words ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, word := range words {
		delete(f.words, strings.ToLower(word))
	}
}

// AddPattern adds a regex pattern to match
func (f *ProfanityFilter) AddPattern(pattern string) error {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.patterns = append(f.patterns, re)
	return nil
}

// SetMode sets the filter mode
func (f *ProfanityFilter) SetMode(mode FilterMode) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
}

// SetReplacement sets the replacement string for FilterModeReplace
func (f *ProfanityFilter) SetReplacement(replacement string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replacement = replacement
}

// Filter checks content for profanity and handles according to mode
func (f *ProfanityFilter) Filter(ctx context.Context, content string, userID string) (filtered string, blocked bool, err error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Check for blocked words
	words := strings.Fields(content)
	hasMatch := false

	for _, word := range words {
		cleanWord := strings.ToLower(stripPunctuation(word))
		if f.words[cleanWord] {
			hasMatch = true
			break
		}
	}

	// Check patterns if no word match
	if !hasMatch {
		for _, pattern := range f.patterns {
			if pattern.MatchString(content) {
				hasMatch = true
				break
			}
		}
	}

	if !hasMatch {
		return content, false, nil
	}

	// Handle based on mode
	switch f.mode {
	case FilterModeBlock:
		return "", true, nil

	case FilterModeMask:
		return f.maskContent(content), false, nil

	case FilterModeReplace:
		return f.replaceContent(content), false, nil

	case FilterModeLog:
		// Log and allow through (logging would be handled elsewhere)
		return content, false, nil

	default:
		return content, false, nil
	}
}

// maskContent replaces blocked words with asterisks
func (f *ProfanityFilter) maskContent(content string) string {
	result := content

	// Mask blocked words
	words := strings.Fields(content)
	for _, word := range words {
		cleanWord := strings.ToLower(stripPunctuation(word))
		if f.words[cleanWord] {
			mask := strings.Repeat("*", len(cleanWord))
			// Use word boundaries for replacement
			re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(cleanWord) + `\b`)
			result = re.ReplaceAllString(result, mask)
		}
	}

	// Mask pattern matches
	for _, pattern := range f.patterns {
		result = pattern.ReplaceAllStringFunc(result, func(match string) string {
			return strings.Repeat("*", len(match))
		})
	}

	return result
}

// replaceContent replaces blocked content with the replacement string
func (f *ProfanityFilter) replaceContent(content string) string {
	result := content

	// Replace blocked words
	for word := range f.words {
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(word) + `\b`)
		result = re.ReplaceAllString(result, f.replacement)
	}

	// Replace pattern matches
	for _, pattern := range f.patterns {
		result = pattern.ReplaceAllString(result, f.replacement)
	}

	return result
}

// stripPunctuation removes common punctuation from a word
func stripPunctuation(word string) string {
	return strings.Trim(word, ".,!?;:'\"()[]{}@#$%^&*-_+=<>/\\|`~")
}

// SpamFilter implements ContentFilter for spam detection
type SpamFilter struct {
	maxCaps         float64 // Maximum percentage of caps (0.0-1.0)
	maxRepeatedChar int     // Maximum repeated characters in a row
	maxUrls         int     // Maximum URLs per message
}

// NewSpamFilter creates a new spam filter with default settings
func NewSpamFilter() *SpamFilter {
	return &SpamFilter{
		maxCaps:         0.7, // 70% caps
		maxRepeatedChar: 5,   // 5 repeated chars
		maxUrls:         3,   // 3 URLs max
	}
}

// Name returns the filter name
func (f *SpamFilter) Name() string {
	return "spam"
}

// Filter checks for spam indicators
func (f *SpamFilter) Filter(ctx context.Context, content string, userID string) (filtered string, blocked bool, err error) {
	// Check for excessive caps
	if f.hasTooManyCaps(content) {
		// Don't block, just convert to lowercase
		return strings.ToLower(content), false, nil
	}

	// Check for repeated characters
	if f.hasRepeatedChars(content) {
		return f.reduceRepeatedChars(content), false, nil
	}

	// Check for too many URLs
	if f.countUrls(content) > f.maxUrls {
		return "", true, nil // Block messages with too many URLs
	}

	return content, false, nil
}

// hasTooManyCaps checks if the message has too many capital letters
func (f *SpamFilter) hasTooManyCaps(content string) bool {
	if len(content) < 10 {
		return false // Skip short messages
	}

	caps := 0
	letters := 0
	for _, r := range content {
		if r >= 'A' && r <= 'Z' {
			caps++
			letters++
		} else if r >= 'a' && r <= 'z' {
			letters++
		}
	}

	if letters == 0 {
		return false
	}

	return float64(caps)/float64(letters) > f.maxCaps
}

// hasRepeatedChars checks for excessive character repetition
func (f *SpamFilter) hasRepeatedChars(content string) bool {
	count := 1
	var prev rune
	for _, r := range content {
		if r == prev {
			count++
			if count > f.maxRepeatedChar {
				return true
			}
		} else {
			count = 1
		}
		prev = r
	}
	return false
}

// reduceRepeatedChars reduces repeated characters to the maximum allowed
func (f *SpamFilter) reduceRepeatedChars(content string) string {
	var result strings.Builder
	count := 1
	var prev rune

	for _, r := range content {
		if r == prev {
			count++
			if count <= f.maxRepeatedChar {
				result.WriteRune(r)
			}
		} else {
			count = 1
			result.WriteRune(r)
		}
		prev = r
	}

	return result.String()
}

// countUrls counts the number of URLs in the content
func (f *SpamFilter) countUrls(content string) int {
	re := regexp.MustCompile(`https?://\S+`)
	return len(re.FindAllString(content, -1))
}

// ChainedFilter combines multiple filters into one
type ChainedFilter struct {
	filters []ContentFilter
}

// NewChainedFilter creates a filter that runs multiple filters in sequence
func NewChainedFilter(filters ...ContentFilter) *ChainedFilter {
	return &ChainedFilter{filters: filters}
}

// Name returns the filter name
func (f *ChainedFilter) Name() string {
	return "chain"
}

// Filter runs all filters in sequence
func (f *ChainedFilter) Filter(ctx context.Context, content string, userID string) (filtered string, blocked bool, err error) {
	current := content
	for _, filter := range f.filters {
		current, blocked, err = filter.Filter(ctx, current, userID)
		if err != nil || blocked {
			return current, blocked, err
		}
	}
	return current, false, nil
}

// NoOpFilter is a filter that does nothing (for testing)
type NoOpFilter struct{}

// Name returns the filter name
func (f *NoOpFilter) Name() string {
	return "noop"
}

// Filter passes content through unchanged
func (f *NoOpFilter) Filter(ctx context.Context, content string, userID string) (filtered string, blocked bool, err error) {
	return content, false, nil
}
