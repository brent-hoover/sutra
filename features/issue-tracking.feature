Feature: Issue tracking

  @slice1
  Scenario: Create an issue
    Given no other input
    When I create an issue with a subject and body
    Then it is stored with a generated id and defaults type=task, status=open, priority=p2, and timestamps set
    Given a new issue is created
    When it is stored
    Then a LedgerEntry of kind created is appended
    Given a missing body
    When I try to create the issue
    Then it is rejected
    Given a missing subject
    When I try to create the issue
    Then it is rejected

  @slice3
  Scenario: List issues
    Given several live issues exist
    When I list issues
    Then all live issues are returned
    Given one of those issues is soft-deleted
    When I list issues
    Then the soft-deleted one is excluded by default

  @slice1
  Scenario: View an issue's core fields
    Given a stored issue exists
    When I view it by id
    Then its core fields are returned

  @slice4
  Scenario: View an issue
    Given an issue with comments, labels, and links
    When I view it by id
    Then its fields, labels, related and blocking links, and comments are shown

  @slice3
  Scenario: Update issue fields
    Given an open issue
    When I change its status to in_progress
    Then the field updates and updated_at advances
    Given a field on an issue changes
    When the change is saved
    Then a LedgerEntry of kind status_changed with field, old_value, and new_value is appended

  @slice3
  Scenario: Comment on an issue
    Given an issue
    When I add a comment with a body
    Then a Comment is stored and a LedgerEntry of kind commented is appended

  @slice3
  Scenario: Soft-delete an issue
    Given an issue
    When I delete it
    Then deleted_at is set and it drops from default lists
    Given an issue is deleted
    When the change is saved
    Then a LedgerEntry of kind deleted is appended

  @slice4
  Scenario: Link issues
    Given two issues
    When I set one as another's parent_id
    Then the child and parent relation holds
    Given an issue
    When I set its parent_id to itself
    Then it is rejected
    Given issues A and B where B's parent is A
    When I set A's parent_id to B
    Then it is rejected as a cycle
    Given issues A, B, and C forming a parent chain A to B to C
    When I set A's parent_id to C
    Then it is rejected as a cycle
    Given two issues
    When I mark A as blocked by B
    Then A shows B in blocked_by and B shows A in is_blocking, derived from issue_block
    Given two issues
    When I relate them
    Then each appears in the other's related list

  @slice4
  Scenario: Label an issue
    Given an issue
    When I add a label
    Then it appears in labels
    Given an issue with a label
    When I remove the label
    Then it no longer appears in labels

  @slice3
  Scenario: View an issue's change history
    Given an issue with several changes
    When I view its history
    Then LedgerEntry rows appear in chronological order

  @slice3
  Scenario: Filter issues
    Given issues with varied status, type, priority, labels, and owner
    When I list with a filter such as status=open, label=bug, or owner=AGENT
    Then only matching, non-soft-deleted issues are returned
    Given multiple filters
    When I combine them
    Then results match all filters using AND
