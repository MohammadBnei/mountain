// Page load order (main.js, styleboard, gate page): the models that have landed, in draw order. Orchestrator-owned
// and NOT frozen: a lane's model joins here when it lands (gate page fails on a built model missing from it).
// Every model's profile is already in the frozen layout.js.
export const page = ['mountain', 'eruption', 'sky', 'cauldron', 'lightning'];
