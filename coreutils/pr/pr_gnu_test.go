// Test vectors of the GNU coreutils pr test suite (tests/pr/pr-tests.pl,
// coreutils 9.4), with their input and expected output fixtures embedded.
// Every vector runs with the common prefix of that suite,
// --date-format="-- Date/Time --" -h x, which makes the page headers
// deterministic.
package pr

import (
	"bytes"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

// The input fixtures of the ported test vectors. Each one holds a page
// worth of numbered lines, with form feed arrangements noted in the text.

var gnuInFFn = "1   FF-Test: FF's in Text                      V\n2   Options -b -3 / -a -3 / ...\n3   --------------------------------------------\n4 3456789 123456789 123456789 123456789 12345678\n5   3 Columns downwards ..., <= 5 lines per page\n6   FF-Arangements: One Empty Page\n7   text\\f\\f\\n; text\\f\\n\\ftext; \\f\\ftext;\n8   \\f\\f\\n; \\f\\n\\f\\n;\n9   \n10  zzzzzzzzzzzzzzzzzzzzzzzzzz123456789\n1                                       12345678\n2                                       12345678\n3 line truncation before FF; r_r_o_l-test:\n14 456789 123456789 123456789 123456789\n\f\f\n15 xyzxyzxyz XYZXYZXYZ abcabcab\n16 456789 123456789 xyzxyzxyz XYZXYZXYZ\n7                                       12345678\n8                                       12345678\n9 3456789 ab\n20 DEFGHI 123\n1                                       12345678\n2                                       12345678\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n27 no truncation before FF; (r_l-test):\n28 no trunc\n\f\f\n29 xyzxyzxyz XYZXYZXYZ abcabcab\n30 456789 123456789 xyzxyzxyz XYZXYZXYZ\n1                                       12345678\n2 3456789 abcdefghi\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9 3456789 abcdefghi\n40 DEFGHI 123456789\n41 yzxyzxyz XYZXYZXYZ abcabcab \n42 456789 123456789 abcdefghi ABCDEDFHI\n\f\f\f\n43 xyzxyzxyz XYZXYZXYZ abcabcab\n44 456789 123456789 xyzxyzxyz XYZXYZXYZ\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9                                       12345678\n50                                      12345678\n1                                       12345678\n2                                       12345678\n3                                       12345678\n4                                       12345678\n55 yzxyzxyz XYZXYZXYZ abcabcab \n56 456789 123456789 abcdefghi ABCDEDFHI\n\f\n57 xyzxyzxyz XYZXYZXYZ abcabcab\n58 456789 123456789 xyzxyzxyz XYZXYZXYZ\n9                                       12345678\n60 DEFGHI 123456789\n"

var gnuIntFFt = "1   FF-Test: FF's in Text                      V\n2   Options -b -3 / -a -3 / ...\n3   --------------------------------------------\n4 3456789 123456789 123456789 123456789 12345678\n5   3 Columns downwards ..., <= 5 lines per page\n6   FF-Arangements: One Empty Page\n7   text\\f\\f\\n; text\\f\\n\\ftext; \\f\\ftext;\n8   \\f\\f\\n; \\f\\n\\f\\n;\n9   \n10  zzzzzzzzzzzzzzzzzzzzzzzzzz123456789\n1                                       12345678\n2                                       12345678\n3 line truncation before FF; r_r_o_l-test:\n14 456789 123456789 123456789 123456789\f\f15 xyzxyzxyz XYZXYZXYZ abcabcab\n16 456789 123456789 xyzxyzxyz XYZXYZXYZ\n7                                       12345678\n8                                       12345678\n9 3456789 ab\n20 DEFGHI 123\n1                                       12345678\n2                                       12345678\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n27 no truncation before FF; (r_l-test):\n28 no trunc\f\f29 xyzxyzxyz XYZXYZXYZ abcabcab\n30 456789 123456789 xyzxyzxyz XYZXYZXYZ\n1                                       12345678\n2 3456789 abcdefghi\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9 3456789 abcdefghi\n40 DEFGHI 123456789\n41 yzxyzxyz XYZXYZXYZ abcabcab \n42 456789 123456789 abcdefghi ABCDEDFHI\f\f\f43 xyzxyzxyz XYZXYZXYZ abcabcab\n44 456789 123456789 xyzxyzxyz XYZXYZXYZ\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9                                       12345678\n50                                      12345678\n1                                       12345678\n2                                       12345678\n3                                       12345678\n4                                       12345678\n55 yzxyzxyz XYZXYZXYZ abcabcab \n56 456789 123456789 abcdefghi ABCDEDFHI\f57 xyzxyzxyz XYZXYZXYZ abcabcab\n58 456789 123456789 xyzxyzxyz XYZXYZXYZ\n9                                       12345678\n60 DEFGHI 123456789\n"

var gnuIntFFtbl = "1   FF-Test: FF's in Text                      V\n2   Options -n;\n3   numbering lines with skiped pages;\n4   numbering blanc lines (no. 5,12,13,23,28)\n\n6 3456789 123456789 123456789 123456789 12345678\n7   3 Columns downwards ..., <= 5 lines per page\n8   \n9   \n10  zzzzzzzzzzzzzzzzzzzzzzzzzz123456789\n1                                       12345678\n          \n\n14 456789 123456789 123456789 123456789\f\f15 xyzxyzxyz XYZXYZXYZ abcabcab\n16 456789 123456789 xyzxyzxyz XYZXYZXYZ\n7                                       12345678\n8                                       12345678\n9 3456789 ab\n20 DEFGHI 123\n1                                       12345678\n\n\n4                                       12345678\n5                                       12345678\n6                                       12345678\n27 no truncation before FF; (r_l-test):\n\n\f\f\n29 xyzxyzxyz XYZXYZXYZ abcabcab\n30 456789 123456789 xyzxyzxyz XYZXYZXYZ\n1                                       12345678\n2 3456789 abcdefghi\n3                                       12345678\n4                                       12345678\n\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9 3456789 abcdefghi\n40 DEFGHI 123456789\n41 yzxyzxyz XYZXYZXYZ abcabcab \n42 456789 123456789 abcdefghi ABCDEDFHI\f\f\f43 xyzxyzxyz XYZXYZXYZ abcabcab\n44 456789 123456789 xyzxyzxyz XYZXYZXYZ\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9                                       12345678\n50                                      12345678\n1                                       12345678\n\n\n\n55 yzxyzxyz XYZXYZXYZ abcabcab \n56 456789 123456789 abcdefghi ABCDEDFHI\f \n58 456789 123456789 xyzxyzxyz XYZXYZXYZ\n9                                       12345678\n60 DEFGHI 123456789\n61 yzxyzxyz XYZXYZXYZ abcabcab \n62 456789 123456789 abcdefghi ABCDEDFHI\n63 xyzxyzxyz XYZXYZXYZ abcabcab\n64 456789 123456789 xyzxyzxyz XYZXYZXYZ\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9                                       12345678\n70  456789 123456789 abcdefghi ABCDEDFHI\n1                                       12345678\n\n\n\n74 yzxyzxyz XYZXYZXYZ abcabcab \n75 456789 123456789 abcdefghi ABCDEDFHI\f\n"

var gnuIntFFtll = "1<<<  -Test: FF's in Text                    >>>\n2<<<  -b -3 / -a -3 / ...  >>>\n3<<<  >>>\n4<<<  123456789 123456789 123456789 123456789 123456789 123456789 123456789  >>>\n\n6<<<  -Arangements: One Empty Page  >>>\n7<<<  \\f\\f\\n; text\\f\\n\\ftext; \\f\\ftext;  >>>\n8<<<  f\\f\\n; \\f\\n\\f\\n;  >>>\n9<<<  >>> \n10<<<  >>>\n1<<<  >>>\n2<<<  >>>\n3<<<  truncation before FF; r_r_o_l-test:  >>>\n14<<<  123456789 123456789 123456789  >>>\f\f15<<<  xyzxyzxyz XYZXYZXYZ abcabcab  >>>\n16<<<  123456789 xyzxyzxyz XYZXYZXYZ  >>>\n7<<<  >>>\n8<<<  >>>\n9<<<  >>>\n20<<<  >>>\n1<<<  >>>\n\n\n4<<<  >>>\n5<<<  >>>\n6<<<  >>>\n27<<<  truncation before FF; (r_l-test):  >>>\n28<<<  trunc\f\f29<<<xyzxyzxyz XYZXYZXYZ abcabcab  >>>\n30<<<  123456789 xyzxyzxyz XYZXYZXYZ  >>>\n1<<<  >>>\n2<<<  abcdefghi  >>>\n3<<<  >>>\n4<<<  >>>\n5<<<  >>>\n6<<<  >>>\n7<<<  >>>\n8<<<  >>>\n9<<<  abcdefghi  >>>\n40<<<  123456789  >>>\n41<<<  XYZXYZXYZ abcabcab   >>>\n42<<<  123456789 abcdefghi ABCDEDFHI  >>>\f\f\f43<<<  xyzxyzxyz XYZXYZXYZ abcabcab  >>>\n44<<<  123456789 xyzxyzxyz XYZXYZXYZ  >>>\n5<<<  >>>\n6<<<  >>>\n7<<<  >>>\n8<<<  >>>\n9<<<  >>>\n50<<<  >>>\n1<<<  >>>\n2<<<  >>>\n3<<<  >>>\n4<<<  >>>\n55<<<  XYZXYZXYZ abcabcab  >>>\n56<<<  123456789 abcdefghi ABCDEDFHI  >>>\f57<<<  xyzxyzxyz XYZXYZXYZ abcabcab  >>>\n58<<<  123456789 xyzxyzxyz XYZXYZXYZ  >>>\n9<<<  >>>\n60<<<  123456789  >>>\n"

var gnuIntn = "1   Test-INPUT: \"Without FF set by Hand\"       V\n2   Options -b -3 [+2|+3] [-l 15|8] [-f]\n3   Options -a -3 [+2|+3] [-l 15|8] [-f]\n4   Options       [+2|+3] [-l 24|17] [-f]\n5\n6   --------------------------------------------\n7 3456789 123456789 123456789 123456789 12345678\n8   3 Columns downwards, across, ...:\n9   With columns use  <= 5 text lines/page,\n10  without -f e.g.:  -l 15 = total lines/page,\n1   with -f e.g.   :  -l 8 -f\n2   \n3 line truncation before new page; r_r_o_l-test:\n14 456789 123456789 123456789 123456789\n15 xyzxyzxyz XYZXYZXYZ abcabcab\n16 456789 123456789 xyzxyzxyz XYZXYZXYZ\n7                                       12345678\n8                                       12345678\n9 3456789 ab\n20 DEFGHI 123\n1                                       12345678\n2                                       12345678\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n27 no truncation before nwe page; (r_l-test):\n28 no trunc\n29 xyzxyzxyz XYZXYZXYZ abcabcab\n30 456789 123456789 xyzxyzxyz XYZXYZXYZ\n1                                       12345678\n2 3456789 abcdefghi\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9 3456789 abcdefghi\n40 DEFGHI 123456789\n41 yzxyzxyz XYZXYZXYZ abcabcab \n42 456789 123456789 abcdefghi ABCDEDFHI\n43 xyzxyzxyz XYZXYZXYZ abcabcab\n44 456789 123456789 xyzxyzxyz XYZXYZXYZ\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9                                       12345678\n50                                      12345678\n1                                       12345678\n2                                       12345678\n3                                       12345678\n4                                       12345678\n55 yzxyzxyz XYZXYZXYZ abcabcab \n56 456789 123456789 abcdefghi ABCDEDFHI\n57 xyzxyzxyz XYZXYZXYZ abcabcab\n58 456789 123456789 xyzxyzxyz XYZXYZXYZ\n9                                       12345678\n60 DEFGHI 123456789\n1\n2\n3  ------- EOF -------- EOF -------- EOF -------\n"

// gnuTest is one test vector of the GNU pr test suite.
type gnuTest struct {
	name string   // GNU test name
	args []string // options, without the common prefix
	in   []string // input fixture names
	desc string   // what the vector covers
	want string   // expected output
}

var gnuTests = []gnuTest{
	{
		"4a",
		[]string{"-l", "24"},
		[]string{"tn"},
		"a file is paginated with the GNU header layout", gnul24t,
	},
	{
		"6a",
		[]string{"-t", "-l", "24"},
		[]string{"tn"},
		"-t omits the header and the trailer", gnutt,
	},
	{
		"7e",
		[]string{"-T", "-l", "24"},
		[]string{"FFn"},
		"-T omits the pagination and drops the form feeds", gnuttFF,
	},
	{
		"6e",
		[]string{"-t", "-l", "24"},
		[]string{"FFn"},
		"-t keeps the form feeds of the input", gnutFF,
	},
	{
		"3la",
		[]string{"-l", "24"},
		[]string{"FFn"},
		"form feeds cause page breaks, and empty pages", gnul24FF,
	},
	{
		"3Pg",
		[]string{"+4:7", "-l", "24"},
		[]string{"tFFt"},
		"a +FIRST:LAST page range", gnu47l24FF,
	},
	{
		"12aa",
		[]string{"-o3", "-a", "-3", "-l24", "-f"},
		[]string{"tn"},
		"a left margin with columns across", gnuo3a3l24ftn,
	},
	{
		"12ab",
		[]string{"-o3", "-a", "-3", "-S:--:", "-l24", "-f"},
		[]string{"tn"},
		"a multi character -S separator", gnuo3a3Sl24ftn,
	},
	{
		"12ac",
		[]string{"-o3", "-a", "-3", "-S:--:", "-n.", "-l24", "-f"},
		[]string{"tn"},
		"a -S separator with line numbers", gnuo3a3Snl24ftn,
	},
	{
		"12ba",
		[]string{"-o3", "-3", "-l24", "-f"},
		[]string{"tn"},
		"a left margin with columns down", gnuo3b3l24ftn,
	},
	{
		"9Pc",
		[]string{"-n.3", "-N", "1", "+3", "-l", "24", "-f"},
		[]string{"tFFt-bl"},
		"-N resets the line numbering of a +page range", gnunN13l24fbl,
	},
	{
		"10wa",
		[]string{"-W", "72", "-J", "-l24", "-f"},
		[]string{"tFFt-ll"},
		"-W with -J joins the lines without truncation", gnuW72Jl24fll,
	},
	{
		"10wc",
		[]string{"-W", "72", "-l24", "-f"},
		[]string{"tFFt-ll"},
		"-W truncates single column lines", gnuW72l24fll,
	},
	{
		"10wh",
		[]string{"-W", "20", "-l24", "-f"},
		[]string{"tFFt-ll"},
		"-W 20 truncates single column lines", gnuW20l24fll,
	},
}

var gnul24t = "\n\n-- Date/Time --                         x                         Page 1\n\n\n1   Test-INPUT: \"Without FF set by Hand\"       V\n2   Options -b -3 [+2|+3] [-l 15|8] [-f]\n3   Options -a -3 [+2|+3] [-l 15|8] [-f]\n4   Options       [+2|+3] [-l 24|17] [-f]\n5\n6   --------------------------------------------\n7 3456789 123456789 123456789 123456789 12345678\n8   3 Columns downwards, across, ...:\n9   With columns use  <= 5 text lines/page,\n10  without -f e.g.:  -l 15 = total lines/page,\n1   with -f e.g.   :  -l 8 -f\n2   \n3 line truncation before new page; r_r_o_l-test:\n14 456789 123456789 123456789 123456789\n\n\n\n\n\n\n\n-- Date/Time --                         x                         Page 2\n\n\n15 xyzxyzxyz XYZXYZXYZ abcabcab\n16 456789 123456789 xyzxyzxyz XYZXYZXYZ\n7                                       12345678\n8                                       12345678\n9 3456789 ab\n20 DEFGHI 123\n1                                       12345678\n2                                       12345678\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n27 no truncation before nwe page; (r_l-test):\n28 no trunc\n\n\n\n\n\n\n\n-- Date/Time --                         x                         Page 3\n\n\n29 xyzxyzxyz XYZXYZXYZ abcabcab\n30 456789 123456789 xyzxyzxyz XYZXYZXYZ\n1                                       12345678\n2 3456789 abcdefghi\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9 3456789 abcdefghi\n40 DEFGHI 123456789\n41 yzxyzxyz XYZXYZXYZ abcabcab \n42 456789 123456789 abcdefghi ABCDEDFHI\n\n\n\n\n\n\n\n-- Date/Time --                         x                         Page 4\n\n\n43 xyzxyzxyz XYZXYZXYZ abcabcab\n44 456789 123456789 xyzxyzxyz XYZXYZXYZ\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9                                       12345678\n50                                      12345678\n1                                       12345678\n2                                       12345678\n3                                       12345678\n4                                       12345678\n55 yzxyzxyz XYZXYZXYZ abcabcab \n56 456789 123456789 abcdefghi ABCDEDFHI\n\n\n\n\n\n\n\n-- Date/Time --                         x                         Page 5\n\n\n57 xyzxyzxyz XYZXYZXYZ abcabcab\n58 456789 123456789 xyzxyzxyz XYZXYZXYZ\n9                                       12345678\n60 DEFGHI 123456789\n1\n2\n3  ------- EOF -------- EOF -------- EOF -------\n\n\n\n\n\n\n\n\n\n\n\n\n"

var gnutt = "1   Test-INPUT: \"Without FF set by Hand\"       V\n2   Options -b -3 [+2|+3] [-l 15|8] [-f]\n3   Options -a -3 [+2|+3] [-l 15|8] [-f]\n4   Options       [+2|+3] [-l 24|17] [-f]\n5\n6   --------------------------------------------\n7 3456789 123456789 123456789 123456789 12345678\n8   3 Columns downwards, across, ...:\n9   With columns use  <= 5 text lines/page,\n10  without -f e.g.:  -l 15 = total lines/page,\n1   with -f e.g.   :  -l 8 -f\n2   \n3 line truncation before new page; r_r_o_l-test:\n14 456789 123456789 123456789 123456789\n15 xyzxyzxyz XYZXYZXYZ abcabcab\n16 456789 123456789 xyzxyzxyz XYZXYZXYZ\n7                                       12345678\n8                                       12345678\n9 3456789 ab\n20 DEFGHI 123\n1                                       12345678\n2                                       12345678\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n27 no truncation before nwe page; (r_l-test):\n28 no trunc\n29 xyzxyzxyz XYZXYZXYZ abcabcab\n30 456789 123456789 xyzxyzxyz XYZXYZXYZ\n1                                       12345678\n2 3456789 abcdefghi\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9 3456789 abcdefghi\n40 DEFGHI 123456789\n41 yzxyzxyz XYZXYZXYZ abcabcab \n42 456789 123456789 abcdefghi ABCDEDFHI\n43 xyzxyzxyz XYZXYZXYZ abcabcab\n44 456789 123456789 xyzxyzxyz XYZXYZXYZ\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9                                       12345678\n50                                      12345678\n1                                       12345678\n2                                       12345678\n3                                       12345678\n4                                       12345678\n55 yzxyzxyz XYZXYZXYZ abcabcab \n56 456789 123456789 abcdefghi ABCDEDFHI\n57 xyzxyzxyz XYZXYZXYZ abcabcab\n58 456789 123456789 xyzxyzxyz XYZXYZXYZ\n9                                       12345678\n60 DEFGHI 123456789\n1\n2\n3  ------- EOF -------- EOF -------- EOF -------\n"

var gnuttFF = "1   FF-Test: FF's in Text                      V\n2   Options -b -3 / -a -3 / ...\n3   --------------------------------------------\n4 3456789 123456789 123456789 123456789 12345678\n5   3 Columns downwards ..., <= 5 lines per page\n6   FF-Arangements: One Empty Page\n7   text\\f\\f\\n; text\\f\\n\\ftext; \\f\\ftext;\n8   \\f\\f\\n; \\f\\n\\f\\n;\n9   \n10  zzzzzzzzzzzzzzzzzzzzzzzzzz123456789\n1                                       12345678\n2                                       12345678\n3 line truncation before FF; r_r_o_l-test:\n14 456789 123456789 123456789 123456789\n15 xyzxyzxyz XYZXYZXYZ abcabcab\n16 456789 123456789 xyzxyzxyz XYZXYZXYZ\n7                                       12345678\n8                                       12345678\n9 3456789 ab\n20 DEFGHI 123\n1                                       12345678\n2                                       12345678\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n27 no truncation before FF; (r_l-test):\n28 no trunc\n29 xyzxyzxyz XYZXYZXYZ abcabcab\n30 456789 123456789 xyzxyzxyz XYZXYZXYZ\n1                                       12345678\n2 3456789 abcdefghi\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9 3456789 abcdefghi\n40 DEFGHI 123456789\n41 yzxyzxyz XYZXYZXYZ abcabcab \n42 456789 123456789 abcdefghi ABCDEDFHI\n43 xyzxyzxyz XYZXYZXYZ abcabcab\n44 456789 123456789 xyzxyzxyz XYZXYZXYZ\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9                                       12345678\n50                                      12345678\n1                                       12345678\n2                                       12345678\n3                                       12345678\n4                                       12345678\n55 yzxyzxyz XYZXYZXYZ abcabcab \n56 456789 123456789 abcdefghi ABCDEDFHI\n57 xyzxyzxyz XYZXYZXYZ abcabcab\n58 456789 123456789 xyzxyzxyz XYZXYZXYZ\n9                                       12345678\n60 DEFGHI 123456789\n"

var gnutFF = "1   FF-Test: FF's in Text                      V\n2   Options -b -3 / -a -3 / ...\n3   --------------------------------------------\n4 3456789 123456789 123456789 123456789 12345678\n5   3 Columns downwards ..., <= 5 lines per page\n6   FF-Arangements: One Empty Page\n7   text\\f\\f\\n; text\\f\\n\\ftext; \\f\\ftext;\n8   \\f\\f\\n; \\f\\n\\f\\n;\n9   \n10  zzzzzzzzzzzzzzzzzzzzzzzzzz123456789\n1                                       12345678\n2                                       12345678\n3 line truncation before FF; r_r_o_l-test:\n14 456789 123456789 123456789 123456789\n\f\f15 xyzxyzxyz XYZXYZXYZ abcabcab\n16 456789 123456789 xyzxyzxyz XYZXYZXYZ\n7                                       12345678\n8                                       12345678\n9 3456789 ab\n20 DEFGHI 123\n1                                       12345678\n2                                       12345678\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n27 no truncation before FF; (r_l-test):\n28 no trunc\n\f\f29 xyzxyzxyz XYZXYZXYZ abcabcab\n30 456789 123456789 xyzxyzxyz XYZXYZXYZ\n1                                       12345678\n2 3456789 abcdefghi\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9 3456789 abcdefghi\n40 DEFGHI 123456789\n41 yzxyzxyz XYZXYZXYZ abcabcab \n42 456789 123456789 abcdefghi ABCDEDFHI\n\f\f\f43 xyzxyzxyz XYZXYZXYZ abcabcab\n44 456789 123456789 xyzxyzxyz XYZXYZXYZ\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9                                       12345678\n50                                      12345678\n1                                       12345678\n2                                       12345678\n3                                       12345678\n4                                       12345678\n55 yzxyzxyz XYZXYZXYZ abcabcab \n56 456789 123456789 abcdefghi ABCDEDFHI\n\f57 xyzxyzxyz XYZXYZXYZ abcabcab\n58 456789 123456789 xyzxyzxyz XYZXYZXYZ\n9                                       12345678\n60 DEFGHI 123456789\n"

var gnul24FF = "\n\n-- Date/Time --                         x                         Page 1\n\n\n1   FF-Test: FF's in Text                      V\n2   Options -b -3 / -a -3 / ...\n3   --------------------------------------------\n4 3456789 123456789 123456789 123456789 12345678\n5   3 Columns downwards ..., <= 5 lines per page\n6   FF-Arangements: One Empty Page\n7   text\\f\\f\\n; text\\f\\n\\ftext; \\f\\ftext;\n8   \\f\\f\\n; \\f\\n\\f\\n;\n9   \n10  zzzzzzzzzzzzzzzzzzzzzzzzzz123456789\n1                                       12345678\n2                                       12345678\n3 line truncation before FF; r_r_o_l-test:\n14 456789 123456789 123456789 123456789\n\n\n\n\n\n\n\n-- Date/Time --                         x                         Page 2\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n-- Date/Time --                         x                         Page 3\n\n\n15 xyzxyzxyz XYZXYZXYZ abcabcab\n16 456789 123456789 xyzxyzxyz XYZXYZXYZ\n7                                       12345678\n8                                       12345678\n9 3456789 ab\n20 DEFGHI 123\n1                                       12345678\n2                                       12345678\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n27 no truncation before FF; (r_l-test):\n28 no trunc\n\n\n\n\n\n\n\n-- Date/Time --                         x                         Page 4\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n-- Date/Time --                         x                         Page 5\n\n\n29 xyzxyzxyz XYZXYZXYZ abcabcab\n30 456789 123456789 xyzxyzxyz XYZXYZXYZ\n1                                       12345678\n2 3456789 abcdefghi\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9 3456789 abcdefghi\n40 DEFGHI 123456789\n41 yzxyzxyz XYZXYZXYZ abcabcab \n42 456789 123456789 abcdefghi ABCDEDFHI\n\n\n\n\n\n\n\n-- Date/Time --                         x                         Page 6\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n-- Date/Time --                         x                         Page 7\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n-- Date/Time --                         x                         Page 8\n\n\n43 xyzxyzxyz XYZXYZXYZ abcabcab\n44 456789 123456789 xyzxyzxyz XYZXYZXYZ\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9                                       12345678\n50                                      12345678\n1                                       12345678\n2                                       12345678\n3                                       12345678\n4                                       12345678\n55 yzxyzxyz XYZXYZXYZ abcabcab \n56 456789 123456789 abcdefghi ABCDEDFHI\n\n\n\n\n\n\n\n-- Date/Time --                         x                         Page 9\n\n\n57 xyzxyzxyz XYZXYZXYZ abcabcab\n58 456789 123456789 xyzxyzxyz XYZXYZXYZ\n9                                       12345678\n60 DEFGHI 123456789\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n"

var gnu47l24FF = "\n\n-- Date/Time --                         x                         Page 4\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n-- Date/Time --                         x                         Page 5\n\n\n29 xyzxyzxyz XYZXYZXYZ abcabcab\n30 456789 123456789 xyzxyzxyz XYZXYZXYZ\n1                                       12345678\n2 3456789 abcdefghi\n3                                       12345678\n4                                       12345678\n5                                       12345678\n6                                       12345678\n7                                       12345678\n8                                       12345678\n9 3456789 abcdefghi\n40 DEFGHI 123456789\n41 yzxyzxyz XYZXYZXYZ abcabcab \n42 456789 123456789 abcdefghi ABCDEDFHI\n\n\n\n\n\n\n\n-- Date/Time --                         x                         Page 6\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n-- Date/Time --                         x                         Page 7\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n"

var gnuo3a3l24ftn = "   \n\n   -- Date/Time --                         x                         Page 1\n\n\n   1   Test-INPUT: \"Withou 2   Options -b -3 [+2|+ 3   Options -a -3 [+2|+\n   4   Options\t     [+2|+ 5\t\t\t   6   -------------------\n   7 3456789 123456789 123 8   3 Columns downwards 9   With columns use\t <\n   10  without -f e.g.:\t - 1   with -f e.g.   :\t - 2\n   3 line truncation befor 14 456789 123456789 123 15 xyzxyzxyz XYZXYZXYZ\n   16 456789 123456789 xyz 7\t\t\t   8\n   9 3456789 ab\t\t   20 DEFGHI 123\t   1\n   2\t\t\t   3\t\t\t   4\n   5\t\t\t   6\t\t\t   27 no truncation before\n   28 no trunc\t\t   29 xyzxyzxyz XYZXYZXYZ  30 456789 123456789 xyz\n   1\t\t\t   2 3456789 abcdefghi\t   3\n   4\t\t\t   5\t\t\t   6\n   7\t\t\t   8\t\t\t   9 3456789 abcdefghi\n   40 DEFGHI 123456789\t   41 yzxyzxyz XYZXYZXYZ a 42 456789 123456789 abc\n\f   \n\n   -- Date/Time --                         x                         Page 2\n\n\n   43 xyzxyzxyz XYZXYZXYZ  44 456789 123456789 xyz 5\n   6\t\t\t   7\t\t\t   8\n   9\t\t\t   50\t\t\t   1\n   2\t\t\t   3\t\t\t   4\n   55 yzxyzxyz XYZXYZXYZ a 56 456789 123456789 abc 57 xyzxyzxyz XYZXYZXYZ\n   58 456789 123456789 xyz 9\t\t\t   60 DEFGHI 123456789\n   1\t\t\t   2\t\t\t   3  ------- EOF --------\n\f"

var gnuo3a3Sl24ftn = "   \n\n   -- Date/Time --                         x                         Page 1\n\n\n   1   Test-INPUT: \"With:--:2\tOptions -b -3 [+2:--:3\t Options -a -3 [+2\n   4   Options\t     [+2:--:5\t\t\t :--:6\t -----------------\n   7 3456789 123456789 1:--:8\t3 Columns downwar:--:9\t With columns use\n   10  without -f e.g.: :--:1\twith -f e.g.   : :--:2\n   3 line truncation bef:--:14 456789 123456789 1:--:15 xyzxyzxyz XYZXYZXY\n   16 456789 123456789 x:--:7\t\t\t :--:8\n   9 3456789 ab\t\t:--:20 DEFGHI 123\t :--:1\n   2\t\t\t:--:3\t\t\t :--:4\n   5\t\t\t:--:6\t\t\t :--:27 no truncation befo\n   28 no trunc\t\t:--:29 xyzxyzxyz XYZXYZXY:--:30 456789 123456789 x\n   1\t\t\t:--:2 3456789 abcdefghi\t :--:3\n   4\t\t\t:--:5\t\t\t :--:6\n   7\t\t\t:--:8\t\t\t :--:9 3456789 abcdefghi\n   40 DEFGHI 123456789\t:--:41 yzxyzxyz XYZXYZXYZ:--:42 456789 123456789 a\n\f   \n\n   -- Date/Time --                         x                         Page 2\n\n\n   43 xyzxyzxyz XYZXYZXY:--:44 456789 123456789 x:--:5\n   6\t\t\t:--:7\t\t\t :--:8\n   9\t\t\t:--:50\t\t\t :--:1\n   2\t\t\t:--:3\t\t\t :--:4\n   55 yzxyzxyz XYZXYZXYZ:--:56 456789 123456789 a:--:57 xyzxyzxyz XYZXYZXY\n   58 456789 123456789 x:--:9\t\t\t :--:60 DEFGHI 123456789\n   1\t\t\t:--:2\t\t\t :--:3\t------- EOF ------\n\f"

var gnuo3a3Snl24ftn = "   \n\n   -- Date/Time --                         x                         Page 1\n\n\n       1.1   Test-INPUT::--:\t2.2   Options -b :--:\t 3.3   Options -a\n       4.4   Options\t:--:\t5.5\t\t :--:\t 6.6   -----------\n       7.7 3456789 12345:--:\t8.8   3 Columns d:--:\t 9.9   With column\n      10.10  without -f :--:   11.1   with -f e.g:--:\t12.2\n      13.3 line truncati:--:   14.14 456789 12345:--:\t15.15 xyzxyzxyz XY\n      16.16 456789 12345:--:   17.7\t\t :--:\t18.8\n      19.9 3456789 ab\t:--:   20.20 DEFGHI 123\t :--:\t21.1\n      22.2\t\t:--:   23.3\t\t :--:\t24.4\n      25.5\t\t:--:   26.6\t\t :--:\t27.27 no truncatio\n      28.28 no trunc\t:--:   29.29 xyzxyzxyz XY:--:\t30.30 456789 12345\n      31.1\t\t:--:   32.2 3456789 abcde:--:\t33.3\n      34.4\t\t:--:   35.5\t\t :--:\t36.6\n      37.7\t\t:--:   38.8\t\t :--:\t39.9 3456789 abcde\n      40.40 DEFGHI 12345:--:   41.41 yzxyzxyz XYZ:--:\t42.42 456789 12345\n\f   \n\n   -- Date/Time --                         x                         Page 2\n\n\n      43.43 xyzxyzxyz XY:--:   44.44 456789 12345:--:\t45.5\n      46.6\t\t:--:   47.7\t\t :--:\t48.8\n      49.9\t\t:--:   50.50\t\t :--:\t51.1\n      52.2\t\t:--:   53.3\t\t :--:\t54.4\n      55.55 yzxyzxyz XYZ:--:   56.56 456789 12345:--:\t57.57 xyzxyzxyz XY\n      58.58 456789 12345:--:   59.9\t\t :--:\t60.60 DEFGHI 12345\n      61.1\t\t:--:   62.2\t\t :--:\t63.3  ------- EOF\n\f"

var gnuo3b3l24ftn = "   \n\n   -- Date/Time --                         x                         Page 1\n\n\n   1   Test-INPUT: \"Withou 15 xyzxyzxyz XYZXYZXYZ  29 xyzxyzxyz XYZXYZXYZ\n   2   Options -b -3 [+2|+ 16 456789 123456789 xyz 30 456789 123456789 xyz\n   3   Options -a -3 [+2|+ 7\t\t\t   1\n   4   Options\t     [+2|+ 8\t\t\t   2 3456789 abcdefghi\n   5\t\t\t   9 3456789 ab\t\t   3\n   6   ------------------- 20 DEFGHI 123\t   4\n   7 3456789 123456789 123 1\t\t\t   5\n   8   3 Columns downwards 2\t\t\t   6\n   9   With columns use\t < 3\t\t\t   7\n   10  without -f e.g.:\t - 4\t\t\t   8\n   1   with -f e.g.   :\t - 5\t\t\t   9 3456789 abcdefghi\n   2\t\t\t   6\t\t\t   40 DEFGHI 123456789\n   3 line truncation befor 27 no truncation before 41 yzxyzxyz XYZXYZXYZ a\n   14 456789 123456789 123 28 no trunc\t\t   42 456789 123456789 abc\n\f   \n\n   -- Date/Time --                         x                         Page 2\n\n\n   43 xyzxyzxyz XYZXYZXYZ  50\t\t\t   57 xyzxyzxyz XYZXYZXYZ\n   44 456789 123456789 xyz 1\t\t\t   58 456789 123456789 xyz\n   5\t\t\t   2\t\t\t   9\n   6\t\t\t   3\t\t\t   60 DEFGHI 123456789\n   7\t\t\t   4\t\t\t   1\n   8\t\t\t   55 yzxyzxyz XYZXYZXYZ a 2\n   9\t\t\t   56 456789 123456789 abc 3  ------- EOF --------\n\f"

var gnunN13l24fbl = "\n\n-- Date/Time --                         x                         Page 3\n\n\n  1.15 xyzxyzxyz XYZXYZXYZ abcabcab\n  2.16 456789 123456789 xyzxyzxyz XYZXYZXYZ\n  3.7                                       12345678\n  4.8                                       12345678\n  5.9 3456789 ab\n  6.20 DEFGHI 123\n  7.1                                       12345678\n  8.\n  9.\n 10.4                                       12345678\n 11.5                                       12345678\n 12.6                                       12345678\n 13.27 no truncation before FF; (r_l-test):\n 14.\n\f\n\n-- Date/Time --                         x                         Page 4\n\n\n\n\f\n\n-- Date/Time --                         x                         Page 5\n\n\n 15.29 xyzxyzxyz XYZXYZXYZ abcabcab\n 16.30 456789 123456789 xyzxyzxyz XYZXYZXYZ\n 17.1                                       12345678\n 18.2 3456789 abcdefghi\n 19.3                                       12345678\n 20.4                                       12345678\n 21.\n 22.6                                       12345678\n 23.7                                       12345678\n 24.8                                       12345678\n 25.9 3456789 abcdefghi\n 26.40 DEFGHI 123456789\n 27.41 yzxyzxyz XYZXYZXYZ abcabcab \n 28.42 456789 123456789 abcdefghi ABCDEDFHI\n\f\n\n-- Date/Time --                         x                         Page 6\n\n\n\n\f\n\n-- Date/Time --                         x                         Page 7\n\n\n\n\f\n\n-- Date/Time --                         x                         Page 8\n\n\n 29.43 xyzxyzxyz XYZXYZXYZ abcabcab\n 30.44 456789 123456789 xyzxyzxyz XYZXYZXYZ\n 31.5                                       12345678\n 32.6                                       12345678\n 33.7                                       12345678\n 34.8                                       12345678\n 35.9                                       12345678\n 36.50                                      12345678\n 37.1                                       12345678\n 38.\n 39.\n 40.\n 41.55 yzxyzxyz XYZXYZXYZ abcabcab \n 42.56 456789 123456789 abcdefghi ABCDEDFHI\n\f\n\n-- Date/Time --                         x                         Page 9\n\n\n 43. \n 44.58 456789 123456789 xyzxyzxyz XYZXYZXYZ\n 45.9                                       12345678\n 46.60 DEFGHI 123456789\n 47.61 yzxyzxyz XYZXYZXYZ abcabcab \n 48.62 456789 123456789 abcdefghi ABCDEDFHI\n 49.63 xyzxyzxyz XYZXYZXYZ abcabcab\n 50.64 456789 123456789 xyzxyzxyz XYZXYZXYZ\n 51.5                                       12345678\n 52.6                                       12345678\n 53.7                                       12345678\n 54.8                                       12345678\n 55.9                                       12345678\n 56.70  456789 123456789 abcdefghi ABCDEDFHI\n\f\n\n-- Date/Time --                        x                         Page 10\n\n\n 57.1                                       12345678\n 58.\n 59.\n 60.\n 61.74 yzxyzxyz XYZXYZXYZ abcabcab \n 62.75 456789 123456789 abcdefghi ABCDEDFHI\n\f"

var gnuW72Jl24fll = "\n\n-- Date/Time --                         x                         Page 1\n\n\n1<<<  -Test: FF's in Text                    >>>\n2<<<  -b -3 / -a -3 / ...  >>>\n3<<<  >>>\n4<<<  123456789 123456789 123456789 123456789 123456789 123456789 123456789  >>>\n\n6<<<  -Arangements: One Empty Page  >>>\n7<<<  \\f\\f\\n; text\\f\\n\\ftext; \\f\\ftext;  >>>\n8<<<  f\\f\\n; \\f\\n\\f\\n;  >>>\n9<<<  >>> \n10<<<  >>>\n1<<<  >>>\n2<<<  >>>\n3<<<  truncation before FF; r_r_o_l-test:  >>>\n14<<<  123456789 123456789 123456789  >>>\n\f\n\n-- Date/Time --                         x                         Page 2\n\n\n\n\f\n\n-- Date/Time --                         x                         Page 3\n\n\n15<<<  xyzxyzxyz XYZXYZXYZ abcabcab  >>>\n16<<<  123456789 xyzxyzxyz XYZXYZXYZ  >>>\n7<<<  >>>\n8<<<  >>>\n9<<<  >>>\n20<<<  >>>\n1<<<  >>>\n\n\n4<<<  >>>\n5<<<  >>>\n6<<<  >>>\n27<<<  truncation before FF; (r_l-test):  >>>\n28<<<  trunc\n\f\n\n-- Date/Time --                         x                         Page 4\n\n\n\n\f\n\n-- Date/Time --                         x                         Page 5\n\n\n29<<<xyzxyzxyz XYZXYZXYZ abcabcab  >>>\n30<<<  123456789 xyzxyzxyz XYZXYZXYZ  >>>\n1<<<  >>>\n2<<<  abcdefghi  >>>\n3<<<  >>>\n4<<<  >>>\n5<<<  >>>\n6<<<  >>>\n7<<<  >>>\n8<<<  >>>\n9<<<  abcdefghi  >>>\n40<<<  123456789  >>>\n41<<<  XYZXYZXYZ abcabcab   >>>\n42<<<  123456789 abcdefghi ABCDEDFHI  >>>\n\f\n\n-- Date/Time --                         x                         Page 6\n\n\n\n\f\n\n-- Date/Time --                         x                         Page 7\n\n\n\n\f\n\n-- Date/Time --                         x                         Page 8\n\n\n43<<<  xyzxyzxyz XYZXYZXYZ abcabcab  >>>\n44<<<  123456789 xyzxyzxyz XYZXYZXYZ  >>>\n5<<<  >>>\n6<<<  >>>\n7<<<  >>>\n8<<<  >>>\n9<<<  >>>\n50<<<  >>>\n1<<<  >>>\n2<<<  >>>\n3<<<  >>>\n4<<<  >>>\n55<<<  XYZXYZXYZ abcabcab  >>>\n56<<<  123456789 abcdefghi ABCDEDFHI  >>>\n\f\n\n-- Date/Time --                         x                         Page 9\n\n\n57<<<  xyzxyzxyz XYZXYZXYZ abcabcab  >>>\n58<<<  123456789 xyzxyzxyz XYZXYZXYZ  >>>\n9<<<  >>>\n60<<<  123456789  >>>\n\f"

var gnuW72l24fll = "\n\n-- Date/Time --                         x                         Page 1\n\n\n1<<<  -Test: FF's in Text                    >>>\n2<<<  -b -3 / -a -3 / ...  >>>\n3<<<  >>>\n4<<<  123456789 123456789 123456789 123456789 123456789 123456789 123456\n\n6<<<  -Arangements: One Empty Page  >>>\n7<<<  \\f\\f\\n; text\\f\\n\\ftext; \\f\\ftext;  >>>\n8<<<  f\\f\\n; \\f\\n\\f\\n;  >>>\n9<<<  >>> \n10<<<  >>>\n1<<<  >>>\n2<<<  >>>\n3<<<  truncation before FF; r_r_o_l-test:  >>>\n14<<<  123456789 123456789 123456789  >>>\n\f\n\n-- Date/Time --                         x                         Page 2\n\n\n\n\f\n\n-- Date/Time --                         x                         Page 3\n\n\n15<<<  xyzxyzxyz XYZXYZXYZ abcabcab  >>>\n16<<<  123456789 xyzxyzxyz XYZXYZXYZ  >>>\n7<<<  >>>\n8<<<  >>>\n9<<<  >>>\n20<<<  >>>\n1<<<  >>>\n\n\n4<<<  >>>\n5<<<  >>>\n6<<<  >>>\n27<<<  truncation before FF; (r_l-test):  >>>\n28<<<  trunc\n\f\n\n-- Date/Time --                         x                         Page 4\n\n\n\n\f\n\n-- Date/Time --                         x                         Page 5\n\n\n29<<<xyzxyzxyz XYZXYZXYZ abcabcab  >>>\n30<<<  123456789 xyzxyzxyz XYZXYZXYZ  >>>\n1<<<  >>>\n2<<<  abcdefghi  >>>\n3<<<  >>>\n4<<<  >>>\n5<<<  >>>\n6<<<  >>>\n7<<<  >>>\n8<<<  >>>\n9<<<  abcdefghi  >>>\n40<<<  123456789  >>>\n41<<<  XYZXYZXYZ abcabcab   >>>\n42<<<  123456789 abcdefghi ABCDEDFHI  >>>\n\f\n\n-- Date/Time --                         x                         Page 6\n\n\n\n\f\n\n-- Date/Time --                         x                         Page 7\n\n\n\n\f\n\n-- Date/Time --                         x                         Page 8\n\n\n43<<<  xyzxyzxyz XYZXYZXYZ abcabcab  >>>\n44<<<  123456789 xyzxyzxyz XYZXYZXYZ  >>>\n5<<<  >>>\n6<<<  >>>\n7<<<  >>>\n8<<<  >>>\n9<<<  >>>\n50<<<  >>>\n1<<<  >>>\n2<<<  >>>\n3<<<  >>>\n4<<<  >>>\n55<<<  XYZXYZXYZ abcabcab  >>>\n56<<<  123456789 abcdefghi ABCDEDFHI  >>>\n\f\n\n-- Date/Time --                         x                         Page 9\n\n\n57<<<  xyzxyzxyz XYZXYZXYZ abcabcab  >>>\n58<<<  123456789 xyzxyzxyz XYZXYZXYZ  >>>\n9<<<  >>>\n60<<<  123456789  >>>\n\f"

var gnuW20l24fll = "\n\n-- Date/Time -- x Page 1\n\n\n1<<<  -Test: FF's in\n2<<<  -b -3 / -a -3 \n3<<<  >>>\n4<<<  123456789 1234\n\n6<<<  -Arangements: \n7<<<  \\f\\f\\n; text\\f\n8<<<  f\\f\\n; \\f\\n\\f\\\n9<<<  >>> \n10<<<  >>>\n1<<<  >>>\n2<<<  >>>\n3<<<  truncation bef\n14<<<  123456789 123\n\f\n\n-- Date/Time -- x Page 2\n\n\n\n\f\n\n-- Date/Time -- x Page 3\n\n\n15<<<  xyzxyzxyz XYZ\n16<<<  123456789 xyz\n7<<<  >>>\n8<<<  >>>\n9<<<  >>>\n20<<<  >>>\n1<<<  >>>\n\n\n4<<<  >>>\n5<<<  >>>\n6<<<  >>>\n27<<<  truncation be\n28<<<  trunc\n\f\n\n-- Date/Time -- x Page 4\n\n\n\n\f\n\n-- Date/Time -- x Page 5\n\n\n29<<<xyzxyzxyz XYZXY\n30<<<  123456789 xyz\n1<<<  >>>\n2<<<  abcdefghi  >>>\n3<<<  >>>\n4<<<  >>>\n5<<<  >>>\n6<<<  >>>\n7<<<  >>>\n8<<<  >>>\n9<<<  abcdefghi  >>>\n40<<<  123456789  >>\n41<<<  XYZXYZXYZ abc\n42<<<  123456789 abc\n\f\n\n-- Date/Time -- x Page 6\n\n\n\n\f\n\n-- Date/Time -- x Page 7\n\n\n\n\f\n\n-- Date/Time -- x Page 8\n\n\n43<<<  xyzxyzxyz XYZ\n44<<<  123456789 xyz\n5<<<  >>>\n6<<<  >>>\n7<<<  >>>\n8<<<  >>>\n9<<<  >>>\n50<<<  >>>\n1<<<  >>>\n2<<<  >>>\n3<<<  >>>\n4<<<  >>>\n55<<<  XYZXYZXYZ abc\n56<<<  123456789 abc\n\f\n\n-- Date/Time -- x Page 9\n\n\n57<<<  xyzxyzxyz XYZ\n58<<<  123456789 xyz\n9<<<  >>>\n60<<<  123456789  >>\n\f"

// TestGNUVectors runs the ported vectors of the GNU pr test suite.
func TestGNUVectors(t *testing.T) {
	for _, test := range gnuTests {
		t.Run(test.desc, func(t *testing.T) {
			fsys := fsx.MemFS{}
			for _, in := range test.in {
				fsys[in] = &fsx.MemFile{Data: []byte(gnuInput(in)), ModTime: modTime}
			}
			stdout := new(bytes.Buffer)
			rt := &coreutils.Runtime{
				Stdout: stdout,
				Stderr: new(bytes.Buffer),
				FS:     fsys,
			}
			args := append([]string{"--date-format=-- Date/Time --", "-h", "x"}, test.args...)
			args = append(args, test.in...)
			if err := New(rt).Run(t.Context(), args...); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got := stdout.String(); got != test.want {
				t.Errorf("Run() stdout = %q, want %q", got, test.want)
			}
		})
	}
}

// gnuInput returns the input fixture with the given name.
func gnuInput(name string) string {
	return map[string]string{
		"FFn":     gnuInFFn,
		"tFFt":    gnuIntFFt,
		"tFFt-bl": gnuIntFFtbl,
		"tFFt-ll": gnuIntFFtll,
		"tn":      gnuIntn,
	}[name]
}

// gnuInline holds the smaller test vectors of the GNU pr test suite, whose
// input and output fit inline.
var gnuInline = []struct {
	name     string
	args     []string // options, without the common prefix
	in       []string // operands, each with the next input
	inputs   []string // the input of each operand
	want     string   // expected output
	wantErr  string   // expected diagnostic, when it fails
	wantExit int      // expected exit status
}{
	{
		name:   "ncut-a",
		args:   []string{"-tn2", "-N98"},
		in:     []string{"a"},
		inputs: []string{"y\ny\ny\ny\ny\n"},
		want:   "98\ty\n99\ty\n00\ty\n01\ty\n02\ty\n",
	},
	{
		name:   "ncut-b",
		args:   []string{"-tn:2", "-N98"},
		in:     []string{"a"},
		inputs: []string{"y\ny\ny\ny\ny\n"},
		want:   "98:y\n99:y\n00:y\n01:y\n02:y\n",
	},
	{
		name:   "margin-0",
		args:   []string{"-o", "0"},
		in:     []string{"a"},
		inputs: []string{""},
		want:   "",
	},
	{
		name:   "dbl-sp-a",
		args:   []string{"-d", "-l", "14", "-h", ""},
		in:     []string{"a"},
		inputs: []string{"1\n2\n"},
		want: "\n\n-- Date/Time --                                                   " +
			"Page 1\n\n\n1\n\n2\n\n\n\n\n\n\n",
	},
	{
		name:   "narrow-1",
		args:   []string{"-W1", "-t"},
		in:     []string{"a"},
		inputs: []string{"12345\n"},
		want:   "1\n",
	},
	{
		name:   "col-last",
		args:   []string{"-W3", "-t2"},
		in:     []string{"a"},
		inputs: []string{"a\nb\nc\n"},
		want:   "a c\nb\n",
	},
	{
		name:   "col-02",
		args:   []string{"-W3", "-t", "-02"},
		in:     []string{"a"},
		inputs: []string{"a\nb\nc\n"},
		want:   "a c\nb\n",
	},
	{
		name:   "col-long",
		args:   []string{"-W3", "-t", "-1", "--columns=2"},
		in:     []string{"a"},
		inputs: []string{"a\nb\nc\n"},
		want:   "a c\nb\n",
	},
	{
		name:   "col-2",
		args:   []string{"-W3", "-t", "-4", "--columns=1", "-2"},
		in:     []string{"a"},
		inputs: []string{"a\nb\nc\n"},
		want:   "a c\nb\n",
	},
	{
		name:   "page-range",
		args:   []string{"+1:1", "-2", "-l1", "-s "},
		in:     []string{"a"},
		inputs: []string{"a\nb\nc\n"},
		want:   "a b\n",
	},
	{
		name:     "neg-page",
		args:     []string{"--pages=1:-1"},
		wantErr:  "pr: invalid page range '1:-1'\nTry 'pr --help' for more information.\n",
		wantExit: 1,
	},
	{
		name:   "padding3",
		args:   []string{"-t", "-N1000000", "-n,1"},
		in:     []string{"a"},
		inputs: []string{"1\n"},
		want:   "0,1\n",
	},
	{
		name:   "merge-w-tabs",
		args:   []string{"-m", "-s", "-t"},
		in:     []string{"a", "b", "c"},
		inputs: []string{"a\tb\tc\n", "m\tn\to\n", "x\ty\tz\n"},
		want:   "a\tb\tc\tm\tn\to\tx\ty\tz\n",
	},
	{
		name:   "merge-2-w-tabs",
		args:   []string{"-m", "-s", "-t"},
		in:     []string{"a", "b"},
		inputs: []string{"a\tb\tc\n", "m\tn\to\n"},
		want:   "a\tb\tc\tm\tn\to\n",
	},
	{
		name:   "asan1",
		args:   []string{"-m", "-S\t\t\t", "-t"},
		in:     []string{"a", "b"},
		inputs: []string{"a\n", "a\n"},
		want:   "a\t\t\t\t  \t\t\ta\n",
	},
}

// TestGNUInline runs the inline test vectors of the GNU pr test suite.
func TestGNUInline(t *testing.T) {
	for _, test := range gnuInline {
		t.Run(test.name, func(t *testing.T) {
			fsys := fsx.MemFS{}
			for i, name := range test.in {
				fsys[name] = &fsx.MemFile{Data: []byte(test.inputs[i]), ModTime: modTime}
			}
			args := append([]string{"--date-format=-- Date/Time --", "-h", "x"}, test.args...)
			args = append(args, test.in...)
			testutil.Once(t, New, testutil.Case{
				FS:       fsys,
				Args:     args,
				Want:     test.want,
				WantErr:  test.wantErr,
				WantExit: test.wantExit,
			})
		})
	}
}
