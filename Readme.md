# LIS2-A - Marshal and Unmarshal

Encode and decode LIS2-A and LIS2-A2 files. Field and segment specification is still incomplete.

## Behavior

 * `Decode` takes a single message held in memory. Callers reading from a network or
   device should limit the message size they accept.
 * Segments are grouped strictly by default: a segment that is out of order (such as a
   record after `L`) or that is missing the segment that starts its group (such as an `R`
   with no preceding `O`) is an error. Set `DecodeOption.LenientGroup` to instead create
   empty groups as needed.
 * Date-times without a UTC offset are read in `DecodeOption.Location` (UTC if unset).
   `EncodeOption.Location` and `EncodeOption.ZoneOffset` control how date-times are
   written; date-only values such as a date of birth are never converted.
 * Decoded strings are always UTF-8. By default a message that is valid UTF-8 is read as
   UTF-8, and any other message is read as Windows-1252 (a superset of the Latin-1 named
   by LIS2-A2). The encoder writes Windows-1252 by default and returns an error for
   characters it cannot represent. Set `Charset` on either option to force Windows-1252,
   Latin-1, or UTF-8.
 * The encoder returns an error for values containing control characters, including CR
   and LF, as they cannot be escaped and would otherwise start a new record.
 * Errors report the line, segment, and field, but not field values, so they may be
   logged without exposing patient data.
 * A `Decoder` may be used concurrently. An `Encoder` must not be.
