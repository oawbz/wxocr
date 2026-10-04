# Reading-order fixtures

`public_046.jpg` is a public Chinese registration form. The expected index sequence was manually read from its visible rows and cells, independently of native OCR output. Its saved OCR boxes/text are only input to the layout test; this fixture does not certify transcription accuracy. Source metadata and image SHA-256 are in `public_046.truth.json`.

`public_047.jpg` adds manually checked pairwise order constraints on a second form with interrupted row rules and merged cells. These constraints cover clear labels and values, not its faint overlapping background print.

`public_014.image` is a poster and `public_083.image` contains shop photographs. Their saved boxes are negative tests: backgrounds, illustrations and window frames must not trigger table ordering. The file suffix does not affect decoding. Sources are in `negative_sources.json`; image SHA-256 values are retained in their original corpus metadata.

Synthetic tests also cover underlines without borders, nonzero image origins and all four upright orientations. The existing dark terminal image is an additional negative test. The table rule requires at least six long horizontal rulings, both side borders, a light background and text distributed across rows and cells. Borderless tables, substantial skew, complex merged cells and dense graphical layouts are not fully supported by this rule.
