use strict;
use warnings;

sub factorial {
    my ($n) = @_;
    return 1 if $n <= 1;
    return $n * factorial($n - 1);
}

sub print_table {
    my ($max) = @_;
    for my $i (1 .. $max) {
        printf "%2d! = %d\n", $i, factorial($i);
    }
}

print_table(10);
