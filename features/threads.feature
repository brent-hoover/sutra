Feature: Threads

  # A Thread is a meta-object that ties together the pieces of one line of work —
  # issues, documents, transcripts, and comments — so the full context lives in
  # one place. Membership is many-to-many; a thread may be scoped to a project.

  @slice11
  Scenario: Create a thread
    Given a title
    When I create a thread
    Then it is stored with a generated id, status active, and timestamps set

  @slice11
  Scenario: Attach items of every kind and view them together
    Given a thread and an issue, a document, a transcript, and a comment
    When I attach each item to the thread
    Then viewing the thread shows all four grouped by kind

  @slice11
  Scenario: Attaching a nonexistent item is rejected
    Given a thread
    When I attach an item id that does not exist
    Then the attach is rejected

  @slice11
  Scenario: An item can belong to multiple threads
    Given two threads and one issue
    When I attach the issue to both threads
    Then the issue appears in both

  @slice11
  Scenario: Detach an item from a thread
    Given a thread with an attached issue
    When I remove the issue from the thread
    Then the thread no longer lists it and the issue still exists

  @slice11
  Scenario: Deleting a thread leaves its members intact
    Given a thread with an attached issue
    When I delete the thread
    Then the thread is gone and the issue still exists

  @slice11
  Scenario: Scope a thread to a project and list by it
    Given a project for the thread
    When I create a thread in that project
    Then the thread lists that project and appears when filtering by it
