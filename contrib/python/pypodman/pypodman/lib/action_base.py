"""Base class for all actions of remote client."""
import abc
from functools import lru_cache

import podman


class AbstractActionBase(abc.ABC):
    """Base class for all actions of remote client."""

    @classmethod
    @abc.abstractmethod
    def subparser(cls, parent):
        """Define parser for this action.  Subclasses must implement.

        API:
        Use set_defaults() to set attributes "class_" and "method". These will
        be invoked as class_(parsed_args).method()
        """
        parent.add_argument(
            '--all',
            action='store_true',
            help=('list all items.'
                  ' (default: no-op, included for compatibility.)'))
        parent.add_argument(
            '--no-trunc',
            '--notruncate',
            action='store_false',
            dest='truncate',
            default=True,
            help='Display extended information. (default: False)')
        parent.add_argument(
            '--noheading',
            action='store_false',
            dest='heading',
            default=True,
            help=('Omit the table headings from the output.'
                  ' (default: False)'))
        parent.add_argument(
            '--quiet',
            action='store_true',
            help='List only the IDs. (default: %(default)s)')

    def __init__(self, args):
        """Construct class."""
        # Dump all unset arguments before transmitting to service
        self._args = args
        self.opts = {
            k: v
            for k, v in vars(self._args).items() if v is not None
        }

    @property
    def remote_uri(self):
        """URI for remote side of connection."""
        return self._args.remote_uri

    @property
    def local_uri(self):
        """URI for local side of connection."""
        return self._args.local_uri

    @property
    def identity_file(self):
        """Key for authenication."""
        return self._args.identity_file

    @property
    def ignore_hosts(self):
        """Ignore ssh host keys."""
        return self._args.ignore_hosts

    @property
    def known_hosts(self):
        """File for known hosts."""
        return self._args.known_hosts

    @property
    @lru_cache(maxsize=1)
    def client(self):
        """Podman remote client for communicating."""
        if self._args.host is None:
            return podman.Client(uri=self.local_uri)
        return podman.Client(
            uri=self.local_uri,
            remote_uri=self.remote_uri,
            identity_file=self.identity_file,
            ignore_hosts=self.ignore_hosts,
            known_hosts=self.known_hosts)

    def __repr__(self):
        """Compute the “official” string representation of object."""
        return ("{}(local_uri='{}', remote_uri='{}',"
                " identity_file='{}', ignore_hosts='{}', known_hosts='{}')").format(
                    self.__class__,
                    self.local_uri,
                    self.remote_uri,
                    self.identity_file,
                    self.ignore_hosts,
                    self.known_hosts,
                )
