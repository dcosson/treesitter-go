### Testing Note

In this project, note that the tests are very slow to run as some of them require compiling all the language grammars and parse large amounts of data.

Scope test runs to just the tests you need, and output results into temporary files so you can parse them instead of grepping for the wrong thing and having to re-run the tests.

Coordinate the full test runs with the scheduler or reviewer agents, who should use the Makefile commands to ensure clean runs before closing out work.
