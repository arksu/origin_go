package ecs

// Only lifecycle transitions enqueue a character; elapsed ticks remain local.
type ActionAnimationDirtyQueue struct{ ObjectBehaviorDirtyQueue }
