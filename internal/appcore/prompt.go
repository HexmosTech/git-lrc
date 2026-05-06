package appcore

const ClaudeHandoffPromptTemplate = `Read %s. This JSON contains code review feedback on my recent changes. The 'hunks' show the current buggy code, and 'comments' explain the issues. Please modify the source files in this workspace to fix the errors mentioned in the comments. Do not treat the 'hunks' as the solution. Briefly explain what you fixed, and then politely remind me that I can type /exit to close this session.`
