# Conventions

Normative schemas that apply to **every cycle and sub-cycle** (AT, CT, Legacy, Structural). The AT cycle is the first to reference them, but sibling-cycle docs point here too.

## Phase scope policy

Each phase doc declares its own allowed-path scope.

**Agent contract on out-of-scope intent.** When a work-agent recognises it needs to edit out of scope, it does **not** wait inline for approval. Instead, it exits with a structured *scope-exception-requested* signal naming the intended out-of-scope file(s) and the reason. The agent's job is to *signal and exit*, never to negotiate inline; the runtime owns what happens next.

A second enforcement layer — a post-phase scripted scope check run by the BPMN runtime — catches anything the agent's signal didn't. Its mechanics and the human-task option set live in the runtime spec, not here.
