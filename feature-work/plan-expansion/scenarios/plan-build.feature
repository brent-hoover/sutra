# scenarios/plan-build.feature
# Maps to: plan.md Step(s) covering plan-tree construction.
#
# Gherkin rules: Given = pre-existing state, When = the single action, Then = observable outcome.
# Each file has: one happy path, one edge case, one failure case.

Feature: Building a plan as a ticket tree
  As an agent
  I want to turn an ordered list of tracer items into a tracked plan tree
  so that the plan is executable tracked work, not throwaway prose.

  Scenario: Building a plan creates a pending plan issue with ordered tracer children
    Given plan prose and an ordered list of 3 tracer items, each with a subject and body
    When I build a plan from them
    Then a plan issue is created with type "plan", approval "pending", and the plan prose as its body
    And 3 child issues are created, one per tracer item, each with its parent_id set to the plan issue
    And the children are chained in order so tracer 1 blocks tracer 2 and tracer 2 blocks tracer 3
    And a LedgerEntry of kind "created" is appended for the plan issue and for each child

  Scenario: Building a plan with a single tracer item creates no blocking edges
    Given plan prose and an ordered list of 1 tracer item
    When I build a plan from it
    Then a plan issue is created with exactly 1 child
    And no blocking edges are created

  Scenario: Building a plan links it to the feature issue it plans when a parent is given
    Given an existing feature issue
    And plan prose and an ordered list of 2 tracer items
    When I build a plan from them referencing the feature issue as parent
    Then the plan issue's parent_id is the feature issue
    And the 2 tracer children have the plan issue as their parent_id

  Scenario: Building a plan is rejected when a tracer item has an empty subject
    Given plan prose and an ordered list of 2 tracer items where the second has an empty subject
    When I try to build a plan from them
    Then the build is rejected as invalid
    And no plan issue and no child issues are created

  Scenario: Building a plan is rejected when a tracer item has an invalid type
    Given plan prose and an ordered list of tracer items where one has type "epic"
    When I try to build a plan from them
    Then the build is rejected as invalid
    And no plan issue and no child issues are created
