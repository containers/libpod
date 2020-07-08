#!/usr/bin/env python3

"""
Verify contents of .cirrus.yml meet specific expectations
"""

import sys
import os
import unittest
import yaml

# Assumes directory structure of this file relative to repo.
SCRIPT_DIRPATH = os.path.dirname(os.path.realpath(__file__))
REPO_ROOT = os.path.realpath(os.path.join(SCRIPT_DIRPATH, '../', '../'))


class TestCaseBase(unittest.TestCase):

    CIRRUS_YAML = None

    def setUp(self):
        with open(os.path.join(REPO_ROOT, '.cirrus.yml')) as cirrus_yaml:
            self.CIRRUS_YAML = yaml.safe_load(cirrus_yaml.read())


class TestDependsOn(TestCaseBase):

    ALL_TASK_NAMES = None

    def setUp(self):
        super().setUp()
        self.ALL_TASK_NAMES = set([key.replace('_task', '')
                                   for key, _ in self.CIRRUS_YAML.items()
                                   if key.endswith('_task')])

    def test_00_dicts(self):
        """Expected dictionaries are present and non-empty"""
        self.assertIn('success_task', self.CIRRUS_YAML)
        self.assertIn('success_task'.replace('_task', ''), self.ALL_TASK_NAMES)
        self.assertIn('depends_on', self.CIRRUS_YAML['success_task'])
        self.assertGreater(len(self.CIRRUS_YAML['success_task']['depends_on']), 0)

    def test_01_depends(self):
        """Success task depends on all other tasks"""
        success_deps = set(self.CIRRUS_YAML['success_task']['depends_on'])
        for task_name in self.ALL_TASK_NAMES - set(['success']):
            with self.subTest(task_name=task_name):
                msg=('Please add "{0}" to the "depends_on" list in "success_task"'
                     "".format(task_name))
                self.assertIn(task_name, success_deps, msg=msg)

    def test_02_skips(self):
        """Every task skips on branches ending with .tmp"""
        for task_name in self.ALL_TASK_NAMES:
            task_block_name=task_name + '_task'
            with self.subTest(task_block_name=task_block_name):
                skip_val='$CIRRUS_BRANCH =~ ".*\.tmp"'
                msg=('Please add "skip: {0}" to the "{1}" block'
                     "".format(skip_val, task_block_name))
                self.assertIn('skip', self.CIRRUS_YAML[task_block_name], msg=msg)
                # Tasks can be skipped for other reasons as well
                self.assertIn(skip_val, self.CIRRUS_YAML[task_block_name].get(
                    'skip', ''), msg=msg)

if __name__ == "__main__":
    unittest.main()
