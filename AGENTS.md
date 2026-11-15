# finwise — Agent Briefing

> Read before every interaction. This file holds only the rules that are already in force; the stack,
> the commands and the gates are added as the project gains them.

## Tracker
- Work is tracked with `br` in `.beads/` (`br ready`, `br show <id>`). A bead is self-contained:
  implement from it, not from an external plan.

## Merging
- An agent merges its own pull request once the repository's gate is green locally and every check
  on the pull request is green; nobody waits for a human to press merge. While the repository has
  no gate and no checks, the pull request body says how the change was verified. Work still goes
  through a branch, atomic commits and a pull request whose body carries the why.
