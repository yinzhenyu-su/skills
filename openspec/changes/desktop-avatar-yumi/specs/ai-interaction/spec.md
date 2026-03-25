## ADDED Requirements

### Requirement: LLM Query Processing
The background daemon SHALL integrate with an LLM provider (e.g., OpenAI) to process questions sent via the CLI.

#### Scenario: Processing an "ask" command
- **WHEN** the daemon receives an `ask` payload from the CLI
- **THEN** it sends the prompt to the configured LLM API and receives a text response

### Requirement: Text-to-Speech (TTS) Generation
The system SHALL convert text strings into playable audio streams using a configured TTS service.

#### Scenario: Synthesizing voice response
- **WHEN** the daemon has a text string to speak (either from an LLM response or a direct `say` command)
- **THEN** it calls the TTS API, downloads the audio data, and triggers the frontend to play it
