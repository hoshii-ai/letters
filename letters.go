package letters

import (
	"fmt"
	"io"
	"net/mail"
	"strings"

	"golang.org/x/net/html/charset"
)

func ParseEmailHeaders(header mail.Header, options ...EmailParserOption) (Headers, bool, error) {
	defaultParser := NewEmailParser(options...)
	headers, anyHeadersSkipped, err := defaultParser.ParseHeaders(header)
	return headers, anyHeadersSkipped, err
}

func ParseHeaders(header mail.Header) (Headers, error) {
	// Deprecated: letters.ParseHeaders exists for backwards compatibility and
	// will be removed in the future. Use letters.NewEmailParser().ParseHeaders
	// or the letters.ParseEmailHeaders helper function instead.
	headers, _, err := ParseEmailHeaders(header)
	return headers, err
}

func ParseEmail(r io.Reader) (Email, bool, error) {
	defaultParser := NewEmailParser()
	return defaultParser.Parse(r)
}

type EmailParser struct {
	bodyFilter           EmailBodyFilter
	fileFilter           EmailFileFilter
	headersParsers       HeadersParsers
	skipMalformedHeaders bool
	skipMalformedParts   bool
}

type EmailParserOption func(*EmailParser)

func DefaultHeadersParsers() HeadersParsers {
	return HeadersParsers{
		Date:               ParseDateHeader,
		Sender:             ParseAddressHeader,
		From:               ParseAddressListHeader,
		ReplyTo:            ParseAddressListHeader,
		To:                 ParseAddressListHeader,
		Cc:                 ParseAddressListHeader,
		Bcc:                ParseAddressListHeader,
		MessageID:          ParseMessageIdHeader,
		InReplyTo:          ParseCommaSeparatedMessageIdHeader,
		References:         ParseCommaSeparatedMessageIdHeader,
		Subject:            ParseStringHeader,
		Comments:           ParseStringHeader,
		Keywords:           ParseCommaSeparatedStringHeader,
		ResentDate:         ParseDateHeader,
		ResentFrom:         ParseAddressListHeader,
		ResentSender:       ParseAddressHeader,
		ResentTo:           ParseAddressListHeader,
		ResentCc:           ParseAddressListHeader,
		ResentBcc:          ParseAddressListHeader,
		ResentMessageID:    ParseMessageIdHeader,
		ContentType:        ParseContentTypeHeader,
		ContentDisposition: ParseContentDisposition,
		ExtraHeaders:       make(map[string]parseStringHeaderFn),
	}
}

func NewEmailParser(options ...EmailParserOption) *EmailParser {
	ep := &EmailParser{
		bodyFilter:     AllBodies,
		fileFilter:     AllFiles,
		headersParsers: DefaultHeadersParsers(),
	}

	for _, option := range options {
		option(ep)
	}

	return ep
}

func (ep *EmailParser) Parse(r io.Reader) (Email, bool, error) {
	var (
		email                    Email
		anyHeadersOrPartsSkipped bool
	)

	msg, err := mail.ReadMessage(r)
	if err != nil {
		return email, anyHeadersOrPartsSkipped, fmt.Errorf(
			"letters.EmailParser.Parse: cannot read message: %w",
			err,
		)
	}

	headers, anyHeadersSkipped, err := ep.ParseHeaders(msg.Header)
	anyHeadersOrPartsSkipped = anyHeadersOrPartsSkipped || anyHeadersSkipped
	if err != nil {
		return email, anyHeadersOrPartsSkipped, fmt.Errorf(
			"letters.EmailParser.Parse: cannot parse headers: %w",
			err,
		)
	}

	email = Email{
		Headers: headers,
	}
	encoding, _ := charset.Lookup(email.Headers.ContentType.Params["charset"])
	cte, err := ParseContentTransferEncoding(
		msg.Header.Get("Content-Transfer-Encoding"),
	)
	if err != nil {
		anyHeadersOrPartsSkipped = true
		if !ep.skipMalformedHeaders {
			return email, anyHeadersOrPartsSkipped, fmt.Errorf(
				"letters.EmailParser.Parse: "+
					"cannot parse Content-Transfer-Encoding: %w",
				err,
			)
		}
	}

	if email.Headers.ContentType.ContentType == contentTypeTextPlain {
		if ep.bodyFilter(email.Headers.ContentType) {
			email.Text, err = parseText(msg.Body, encoding, cte)
			if err != nil {
				anyHeadersOrPartsSkipped = true
				if !ep.skipMalformedParts {
					return email, anyHeadersOrPartsSkipped, fmt.Errorf(
						"letters.EmailParser.Parse: "+
							"cannot parse plain text: %w",
						err,
					)
				}
			}
		}
	} else if email.Headers.ContentType.ContentType == contentTypeTextEnriched {
		if ep.bodyFilter(email.Headers.ContentType) {
			email.EnrichedText, err = parseText(msg.Body, encoding, cte)
			if err != nil {
				anyHeadersOrPartsSkipped = true
				if !ep.skipMalformedParts {
					return email, anyHeadersOrPartsSkipped,
						fmt.Errorf(
							"letters.EmailParser.Parse: "+
								"cannot parse enriched text: %w",
							err,
						)
				}
			}
		}
	} else if email.Headers.ContentType.ContentType == contentTypeTextHtml {
		if ep.bodyFilter(email.Headers.ContentType) {
			email.HTML, err = parseText(msg.Body, encoding, cte)
			if err != nil {
				anyHeadersOrPartsSkipped = true
				if !ep.skipMalformedParts {
					return email, anyHeadersOrPartsSkipped,
						fmt.Errorf(
							"letters.EmailParser.Parse: "+
								"cannot parse html text: %w",
							err,
						)
				}
			}
		}
	} else if strings.HasPrefix(
		email.Headers.ContentType.ContentType,
		contentTypeMultipartPrefix,
	) {
		boundary := email.Headers.ContentType.Params["boundary"]
		emailBodies, anyPartsSkipped, err := ep.parsePart(
			msg.Body,
			email.Headers.ContentType,
			boundary,
		)
		anyHeadersOrPartsSkipped = anyHeadersOrPartsSkipped || anyPartsSkipped
		if err != nil {
			if !ep.skipMalformedParts {
				return email, anyHeadersOrPartsSkipped, fmt.Errorf(
					"letters.EmailParser.Parse: "+
						"cannot parse part %q with boundary %q: %w",
					email.Headers.ContentType.ContentType,
					boundary,
					err,
				)
			}
		}
		email.Text = emailBodies.text
		email.EnrichedText = emailBodies.enrichedText
		email.HTML = emailBodies.html
		email.InlineFiles = emailBodies.InlineFiles
		email.AttachedFiles = emailBodies.AttachedFiles
	} else {
		afl, err := decodeAttachmentFileFromBody(msg.Body, email.Headers, cte)
		if err != nil {
			anyHeadersOrPartsSkipped = true
			if !ep.skipMalformedParts {
				return email, anyHeadersOrPartsSkipped, fmt.Errorf(
					"letters.EmailParser.Parse: "+
						"cannot decode attached file content from body: %w",
					err,
				)
			}
		}
		email.AttachedFiles = append(email.AttachedFiles, afl)
	}

	email.Text = normalizeMultilineString(email.Text)
	email.EnrichedText = normalizeMultilineString(email.EnrichedText)
	email.HTML = normalizeMultilineString(email.HTML)

	return email, anyHeadersOrPartsSkipped, nil
}
