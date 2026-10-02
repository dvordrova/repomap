"""Scores a program using the package reads: Scoreboard is its API."""

from fixture_app._formats import score_text


class Scoreboard:
    """A public class of a public module: its public methods are the library's."""

    def show(self, level):
        def padded(text):
            return text.rjust(4)

        return padded(score_text(level))

    def _reset(self):
        return None
