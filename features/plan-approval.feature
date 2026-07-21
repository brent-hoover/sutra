Feature: Approving a plan

  @slice13
  Scenario: Approve a pending plan
    Given a plan issue with approval pending
    When I approve the plan issue
    Then its approval becomes approved and updated_at advances
    Given an approval is recorded
    When the change is saved
    Then a LedgerEntry of kind updated with field approval and new_value approved is appended
    Given a plan issue that is already approved
    When I approve it again
    Then its approval is still approved and no additional LedgerEntry is appended

  @slice13
  Scenario: Approval rejects invalid targets
    Given an issue of type task
    When I try to approve it as a plan
    Then it is rejected and the issue is unchanged
    Given no issue exists with the given id
    When I try to approve that id as a plan
    Then it is rejected as not found
    Given an issue of type task
    When I try to change its type to plan
    Then it is rejected
