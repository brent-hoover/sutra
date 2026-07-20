# scenarios/plan-approval.feature
# Maps to: plan.md Step(s) covering plan approval.
#
# Gherkin rules: Given = pre-existing state, When = the single action, Then = observable outcome.
# Each file has: one happy path, one edge case, one failure case.

Feature: Approving a plan
  As a human reviewer
  I want to approve a proposed plan issue
  so that its tracer items are signed off as live work before an agent starts.

  Background:
    Given a plan issue exists with approval "pending"

  Scenario: Approving a pending plan marks it approved and records it in the ledger
    Given the plan issue's approval is "pending"
    When I approve the plan issue
    Then the plan issue's approval becomes "approved"
    And its updated_at advances
    And a LedgerEntry of kind "updated" with field "approval" and new_value "approved" is appended

  Scenario: Approving an already-approved plan is an idempotent no-op
    Given the plan issue has already been approved
    When I approve the plan issue again
    Then the plan issue's approval is still "approved"
    And no additional LedgerEntry is appended

  Scenario: Approving a non-plan issue is rejected
    Given an issue of type "task"
    When I try to approve it as a plan
    Then the request is rejected as invalid
    And the issue's fields are unchanged

  Scenario: Approving a plan issue that does not exist is rejected as not found
    Given no issue exists with the given id
    When I try to approve that id as a plan
    Then the request is rejected as not found
