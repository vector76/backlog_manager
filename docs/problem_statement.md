I want to make a client/server application in go, but before I get into it too 
much let me give some background.

I have a workflow executor called raymond.  The source code resides at 
../raymond and there is documentation within there at ../raymond/docs.  
Workflows are defined as a collection of markdown files and shell scripts and
raymond evaluates those files like an interpreter executing a program (using 
an invocation of claude code at each step).

I have workflows defined to write code, and the way they work, they pull work
items from an issue tracker called beads server or bs.  The source code resides
at ../beads_server and it has docs.  The coding workflow selects an item (a 
"bead") from the issue tracker using `bs` CLI, works the item, commits and 
pushes to git, and marks the issue as closed.  The workflow defines a looping
structure that then starts over, picking up the next issue and continuing until
there are no more issues to work.

There is some effort involved in generating the issues, and I have written some
workflows to try to make it easier, but it's still one of the more 
labor-intensive areas.  This is where backlog manager would come in.  The 
current workflow (which is not great) starts with an underspecified idea.  I
have a raymond workflow defined that uses claude and queries to the user, in 
the context of the existing codebase to flesh out the idea into a fully 
specified feature that doesn't have ambiguities or inconsistencies.  This 
feature is then transformed into an implementation plan (strategic level) and 
then the plan is broken down into beads.  The breakdown aims for a specific 
quantity of work in each bead so that the worker can implement in a single 
context window.

Right now this transformation from idea -> fully specified -> plan -> beads is
what I want to improve.  These are the disadvantages I want to solve:
1. I have to manually manage the files for the idea/fullspec/plan
2. Dialog with the user is hacked together with shell scripts and it's clumsy
3. It's clumsy to look at the fully specified features while having the dialog

I am envisioning a feature-builder raymond workflow that executes in a folder 
for each project being managed.  This will use the backlog manager (bm) client
in command-line mode to do all the operations needed for the idea-to-beads 
development.  The raymond workflow must run in a folder containing the project
source code to have the context for understanding the feature, and generating 
the plan, etc.

Multiple bm clients (with raymond orchestration) can run in multiple shells and
even across multiple machines or containers, and all talk to a centralized 
server.  The server also offers a web UI that provides the user interface.  
This is a similar structure to the beads server, where the server manages the 
database and offers a dashboard, and the 'bs' clients are purely CLI.  The 
difference is that the bm server will be more interactive and not just for 
viewing.

The user interface should be partitioned such that it's easy to view/hide the 
different projects, but also simple to see them all together.  The beads 
dashboard is a good model for this.

For a given project, the content is structured in terms of a list of features.  
Each feature begins on the web interface with the user creating a new feature 
and assigning a name and a starting feature description.  The description may 
be extremely short, or it could be several paragraphs.  During creation it can 
be edited and saved, and this does not automatically trigger a dialogue.  When 
the user is ready, the user can "start dialog".

The client (which remember resides in the code directory of the project) fetches
the feature description and for the initial invocation, uses nothing else other 
than the codebase for context/input.  The raymond process analyzes the feature 
and produces two things: one, a updated version of the feature, and two, a 
message to the user asking questions or stating assumptions.  These two 
documents are submitted back to the server.

The server then updates the feature description and presents the direct message 
(questions/assumptions) to the user.  The user is expected to answer the direct 
message but may also review the feature document for correctness.  The 
interface allows the user to enter a response.  These three documents, the 
latest feature, the questions/assumptions, and the user response are then made
available to the client.  The client then pulls the three documents and again
produces two documents: an updated feature document and new assumptions/questions.

This loop proceeds until the user marks the answer as "final answer".  After
marking the final answer, the feature is updated one last time and no further 
questions are asked.  The feature is then considered fully specified.

The user may also "reopen" a fully specified feature.  The user submits a 
message, which goes into the same loop as before, where the workflow gets three 
documents: the feature, the questions/assumptions (empty in this case), and the 
user message.  This can loop until the user again marks a final message.

When the feature is fully specified, it does not automatically submit for the
feature -> plan -> beadify -> commit workflow.  The user has an option to 
"generate now", which marks the feature as ready to generate, or the user can 
specify "generate after ___".  This allows queueing of features that are 
implemented sequentially.  For this to work, the backlog manager will need some 
way of registering the beads that are associated with a feature, and must 
monitor those beads to detect when all of them are closed.  This would then 
unblock the subsequent feature.

