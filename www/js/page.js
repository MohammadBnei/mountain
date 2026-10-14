// Page load order (main.js, styleboard, gate page): the models that have landed, in draw order. Orchestrator-owned
// and NOT frozen: a lane's model joins here when it lands (gate page fails on a built model missing from it).
// Every model's profile is already in the frozen layout.js.
// strata (W3a): 0 draws, colours the cut face; the story opens the cut (P 3), the finale keeps it closed.
export const page = ['mountain', 'strata', 'eruption', 'sky', 'cauldron', 'lightning'];
