package openai

func cutCallTool() map[string]any {
	return map[string]any{
		"type": "function",
		"name": "cut_call",
		"description": `Ends the ongoing call gracefully. Follow this process:

STEP 1 - ASK FOR CONFIRMATION:
When user mentions ending the call, first respond with a confirmation like:
- "Would you like me to end the call now?"
- "Should I go ahead and end this call?"
- "Are you ready to hang up, or is there anything else I can help with?"

DO NOT call this tool yet - wait for their response.

STEP 2 - ONLY AFTER CONFIRMATION:
Call this tool ONLY if the user explicitly confirms with responses like:
- "Yes", "Yeah", "Sure", "Okay", "Go ahead"
- "End the call", "Hang up", "That's all", "I'm done"

When calling this tool, provide a warm farewell message in cut_call_message that will be spoken before disconnecting.`,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cut_call_message": map[string]any{
					"type":        "string",
					"description": "The warm farewell message to speak before ending, e.g. 'Thank you for speaking with me today! Take care and have a wonderful day. Goodbye!'",
				},
			},
			"required": []string{"cut_call_message"},
		},
	}
}

func setLanguageTool() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        "set_language",
		"description": "Sets the language preference for the conversation. Call this if the user explicitly asks to change or set the language.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"language": map[string]any{
					"type":        "string",
					"enum":        []string{"english", "hindi"},
					"description": "The language to set. Supported values are 'english' or 'hindi'.",
				},
			},
			"required": []string{"language"},
		},
	}
}

func getTools() []map[string]any {
	return []map[string]any{
		cutCallTool(),
		setLanguageTool(),
	}
}
