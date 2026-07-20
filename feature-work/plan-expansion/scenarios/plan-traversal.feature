# scenarios/plan-traversal.feature
# Maps to: plan.md Step(s) covering child enumeration (parent_id list filter) and working the plan.
#
# Gherkin rules: Given = pre-existing state, When = the single action, Then = observable outcome.
# Each file has: one happy path, one edge case, one failure case.

Feature: Working through a plan
  As an agent
  I want to enumerate a plan's tracer items in order and mark them complete
  so that I can work the plan to completion.

  Background:
    Given an approved plan issue with 3 tracer children chained in order

  Scenario: Listing a plan's tracer items returns them in run order with their status
    Given the plan issue's 3 tracer children are all open
    When I list issues filtered by the plan issue as parent
    Then exactly the 3 tracer children are returned, in run order
    And each returned item includes its current status

  Scenario: Completing a tracer is reflected on the next traversal
    Given the first tracer child is open and blocks the second
    When I close the first tracer child and list the plan's children again
    Then the first tracer child is returned with status "closed"
    And the second tracer child's blocked_by no longer includes an open blocker

  Scenario: Listing an issue that has no children returns an empty set
    Given an issue with no children
    When I list issues filtered by that issue as parent
    Then no issues are returned

  Scenario: Listing a plan's children with an invalid status filter is rejected
    Given the plan issue with its tracer children
    When I list its children with a status filter of "done"
    Then the request is rejected as an invalid filter
