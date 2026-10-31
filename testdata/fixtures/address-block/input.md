```yaml metadata
subject: Invitation to the spring sale
sender: |
  Example Garden Society
  2 Orchard Road
  54321 Exampleton
recipient: |
  Ada Example
  Flat 4, 1 Sample Lane
  12345 Exampleton
  Sampleland
opening: Dear Ms Example,
closing: Kind regards
```
The address block above was written as a multi-line string and prints as four
separate lines, one per source line.

Address-like lines in the body keep their breaks too:

Example Garden Society<br>
2 Orchard Road<br>
54321 Exampleton
