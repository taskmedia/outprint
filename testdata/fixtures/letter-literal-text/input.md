```yaml metadata
subject: 'Costs $x$ and \input{foo} in *plain* text'
recipient:
  - '$5 & 10% of #7'
  - '\textbf{not bold}'
```
Prices such as $5 and $10 appear as written, and so does $x$.

The command \input{foo} is shown literally, as is \textbf{this} and 100%.

A code span `\input{bar}` and the words under_score and ^caret stay as written.
