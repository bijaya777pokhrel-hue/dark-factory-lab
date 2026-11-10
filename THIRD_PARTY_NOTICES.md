# Third-party notices

## VADER sentiment lexicon

- **File:** `lexiscore/vader_lexicon.txt`
- **What it is:** the VADER sentiment lexicon, a list of English words (and some
  emoticons) with human-rated mean valence scores from -4 to 4.
- **Where it is used:** embedded with `//go:embed` in the `lexiscore` package,
  which uses each word's mean valence to score text.
- **Source repository:** https://github.com/cjhutto/vaderSentiment
- **Pinned commit:** `44fc044cd877310ee8278a0eadf34bcd50d41d06`
- **Path in the source repository:** `vaderSentiment/vader_lexicon.txt`
- **Modifications:** included unmodified.
- **SHA-256:** `ec636f884f36b5de6a4d681b2f61c08c2ec90eeb440472033f3307d0d6fa8bc9`
- **Licence:** MIT, Copyright (c) 2016 C.J. Hutto. The full licence text is in
  [`lexiscore/LICENSE-VADER.txt`](lexiscore/LICENSE-VADER.txt).

### Citation

Hutto, C.J. & Gilbert, E.E. (2014). VADER: A Parsimonious Rule-based Model for
Sentiment Analysis of Social Media Text. Eighth International Conference on
Weblogs and Social Media (ICWSM-14). Ann Arbor, MI, June 2014.
