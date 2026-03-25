## ADDED Requirements

### Requirement: State-machine Animation
The system SHALL support transitioning between different bone animations (e.g., idle, thinking, cheering).

#### Scenario: Triggering an animation
- **WHEN** an IPC command requests a "celebrate" action
- **THEN** the model transitions smoothly from the "idle" animation to the "celebrate" animation

### Requirement: Facial Expression Control
The system SHALL allow changing the VRM BlendShapes to convey emotions (e.g., happy, sad, angry).

#### Scenario: Setting an emotion
- **WHEN** an IPC command requests a "happy" emotion
- **THEN** the VRM `preset_joy` BlendShape is activated

### Requirement: Audio-driven Lip-sync
The system SHALL map the amplitude of the currently playing audio to the mouth BlendShapes of the model.

#### Scenario: Model speaking
- **WHEN** an audio file is playing through the system
- **THEN** the avatar's mouth opens and closes in real-time correlation with the audio volume amplitude
