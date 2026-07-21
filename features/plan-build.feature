Feature: Building a plan as a ticket tree

  @slice13
  Scenario: Build a plan into a ticket tree
    Given plan prose and an ordered list of 3 tracer items each with a subject and body
    When I build a plan from them
    Then a plan issue is created with type plan, approval pending, and the prose as its body
    And 3 child issues are created, one per tracer item, each with parent_id set to the plan issue
    And the children are chained in order so tracer 1 blocks tracer 2 and tracer 2 blocks tracer 3
    Given a plan tree is built
    When it is stored
    Then a LedgerEntry of kind created is appended for the plan issue and for each child
    Given plan prose and an ordered list of 1 tracer item
    When I build a plan from it
    Then a plan issue with exactly 1 child is created and no blocking edges exist
    Given an existing feature issue that belongs to a project
    When I build a plan referencing that feature issue as parent
    Then the plan issue's parent_id is the feature issue
    And the plan issue and its children carry the feature issue's project_id
    Given plan prose, tracer items, and an explicit project_id
    When I build a plan with that project_id and no parent
    Then the plan issue and its children carry that project_id
    Given plan prose and tracer items with neither a parent nor a project_id
    When I build a plan from them
    Then the plan issue and its children have no project_id

  @slice13
  Scenario: Building a plan rejects invalid tracer input
    Given plan prose and tracer items where one has an empty subject
    When I try to build the plan
    Then it is rejected
    And no plan issue and no child issues are created
    Given plan prose and tracer items where one has type epic
    When I try to build the plan
    Then it is rejected
    And no plan issue and no child issues are created
    Given plan prose and an empty list of tracer items
    When I try to build the plan
    Then it is rejected
    And no plan issue and no child issues are created
